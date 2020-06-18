package authorizer

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/3scale/3scale-go-client/threescale/api"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/authorizer"
	"github.com/3scale/3scale-porta-go-client/client"
	log "github.com/sirupsen/logrus"
)

type Authorizer struct {
	stopChan chan struct{}
	config   Config
	manager  *authorizer.Manager
}

type Config struct {
	serviceAPICacheConfig *authorizer.SystemCacheConfig
	authorizationAPICache bool
	adminPortalURL        *url.URL
	environment           string
	serviceID             string
}

// TODO: Add a default builder that uses cache by default with everything set to sane defaults.
// TODO: Add a more customizable builder.

func New(AdminPortalURL *url.URL, environment, serviceID string, ServiceAPICache,
	AuthorizationAPICache bool) (*Authorizer, error) {
	var httpClient *http.Client
	var systemCacheConfig authorizer.SystemCacheConfig

	if serviceID == "" {
		return nil, fmt.Errorf("missing serviceID")
	}
	if environment == "" {
		log.Warning("Environment variable is empty, running in sandbox mode.")
		environment = "sandbox"
	}

	//TODO: Match defaults with 3scale-istio-adapter lib
	if ServiceAPICache {
		systemCacheConfig.MaxSize = 100
		systemCacheConfig.NumRetryFailedRefresh = 3
		systemCacheConfig.RefreshInterval = 30 * time.Second
		systemCacheConfig.TTL = 120 * time.Second
	}

	stop := make(chan struct{})

	systemCache := authorizer.NewSystemCache(systemCacheConfig, stop)
	builder := authorizer.NewClientBuilder(httpClient)
	manager, err := authorizer.NewManager(builder, systemCache, AuthorizationAPICache)
	if err != nil {
		return nil, err
	}

	return &Authorizer{
		stopChan: stop,
		manager:  manager,
		config: Config{
			serviceAPICacheConfig: &systemCacheConfig,
			authorizationAPICache: AuthorizationAPICache,
			adminPortalURL:        AdminPortalURL,
			environment:           environment,
			serviceID:             serviceID,
		},
	}, nil
}

func (a *Authorizer) Stop() {
	a.stopChan <- struct{}{}
}

func (a *Authorizer) AuthzRep(request *http.Request) (isAuthorized bool, err error) {

	// Application ID/OpenID Connect authentication pattern - App Key is optional when using this authn
	var appID, appKey string
	// Application Key auth pattern
	var userKey string

	token, _ := a.config.adminPortalURL.User.Password()
	systemRequest := authorizer.SystemRequest{
		AccessToken: token,
		ServiceID:   a.config.serviceID,
		Environment: a.config.environment,
	}

	proxyConfig, err := a.manager.GetSystemConfiguration(a.config.adminPortalURL.String(), systemRequest)
	if err != nil {
		return false, err
	}

	metrics := generateMetrics(request.RequestURI, request.Method, proxyConfig)

	backendURL := proxyConfig.Content.Proxy.Backend.Endpoint

	params := request.URL.Query()
	appID = params.Get(proxyConfig.Content.Proxy.AuthAppID)
	appKey = params.Get(proxyConfig.Content.Proxy.AuthAppKey)
	userKey = params.Get(proxyConfig.Content.Proxy.AuthUserKey)

	backendRequest := authorizer.BackendRequest{
		Auth: authorizer.BackendAuth{
			Type:  proxyConfig.Content.BackendAuthenticationType,
			Value: proxyConfig.Content.BackendAuthenticationValue,
		},
		Service: a.config.serviceID,
		Transactions: []authorizer.BackendTransaction{
			{
				Metrics: metrics,
				Params: authorizer.BackendParams{
					AppID:   appID,
					AppKey:  appKey,
					UserKey: userKey,
				},
			},
		},
	}

	resp, err := a.manager.AuthRep(backendURL, backendRequest)
	if err != nil {
		return false, err
	}
	return resp.Authorized, nil
}

func generateMetrics(path string, method string, conf client.ProxyConfig) api.Metrics {
	metrics := make(api.Metrics)
	for _, pr := range conf.Content.Proxy.ProxyRules {
		if match, err := regexp.MatchString(pr.Pattern, path); err == nil {
			if match && strings.ToUpper(pr.HTTPMethod) == strings.ToUpper(method) {
				metrics.Add(pr.MetricSystemName, int(pr.Delta))
			}
		}
	}
	return metrics
}
