package session

import (
	"crypto/x509"
	"encoding/json"
	"time"

	fasthttpsession "github.com/authelia/session/v2"
	"github.com/authelia/session/v2/providers/memory"
	"github.com/authelia/session/v2/providers/redis"
	"github.com/valyala/fasthttp"

	"github.com/authelia/authelia/internal/configuration/schema"
	"github.com/authelia/authelia/internal/logging"
	"github.com/authelia/authelia/internal/utils"
)

// Provider a session provider.
type Provider struct {
	sessionHolder *fasthttpsession.Session
	RememberMe    time.Duration
	Inactivity    time.Duration
}

// NewProvider instantiate a session provider given a configuration.
func NewProvider(configuration schema.SessionConfiguration, certPool *x509.CertPool) *Provider {
	providerConfig := NewProviderConfig(configuration, certPool)

	provider := new(Provider)
	provider.sessionHolder = fasthttpsession.New(providerConfig.config)

	logger := logging.Logger()

	duration, err := utils.ParseDurationString(configuration.RememberMeDuration)
	if err != nil {
		logger.Fatal(err)
	}

	provider.RememberMe = duration

	duration, err = utils.ParseDurationString(configuration.Inactivity)
	if err != nil {
		logger.Fatal(err)
	}

	provider.Inactivity = duration

	var providerImpl fasthttpsession.Provider

	switch {
	case providerConfig.redisConfig != nil:
		providerImpl, err = redis.New(*providerConfig.redisConfig)
		if err != nil {
			logger.Fatal(err)
		}
	case providerConfig.redisSentinelConfig != nil:
		providerImpl, err = redis.NewFailoverCluster(*providerConfig.redisSentinelConfig)
		if err != nil {
			logger.Fatal(err)
		}
	default:
		providerImpl, err = memory.New(memory.Config{})
		if err != nil {
			logger.Fatal(err)
		}
	}

	err = provider.sessionHolder.SetProvider(providerImpl)
	if err != nil {
		logger.Fatal(err)
	}

	return provider
}

// GetSession return the user session from a request.
func (p *Provider) GetSession(ctx *fasthttp.RequestCtx) (UserSession, error) {
	store, err := p.sessionHolder.Get(ctx)

	if err != nil {
		return NewDefaultUserSession(), err
	}

	userSessionJSON, ok := store.Get(userSessionStorerKey).([]byte)

	// If userSession is not yet defined we create the new session with default values
	// and save it in the store.
	if !ok {
		userSession := NewDefaultUserSession()
		store.Set(userSessionStorerKey, userSession)

		return userSession, nil
	}

	var userSession UserSession
	err = json.Unmarshal(userSessionJSON, &userSession)

	if err != nil {
		return NewDefaultUserSession(), err
	}

	return userSession, nil
}

// SaveSession save the user session.
func (p *Provider) SaveSession(ctx *fasthttp.RequestCtx, userSession UserSession) error {
	store, err := p.sessionHolder.Get(ctx)

	if err != nil {
		return err
	}

	userSessionJSON, err := json.Marshal(userSession)

	if err != nil {
		return err
	}

	store.Set(userSessionStorerKey, userSessionJSON)

	err = p.sessionHolder.Save(ctx, store)

	if err != nil {
		return err
	}

	return nil
}

// RegenerateSession regenerate a session ID.
func (p *Provider) RegenerateSession(ctx *fasthttp.RequestCtx) error {
	err := p.sessionHolder.Regenerate(ctx)
	return err
}

// DestroySession destroy a session ID and delete the cookie.
func (p *Provider) DestroySession(ctx *fasthttp.RequestCtx) error {
	return p.sessionHolder.Destroy(ctx)
}

// UpdateExpiration update the expiration of the cookie and session.
func (p *Provider) UpdateExpiration(ctx *fasthttp.RequestCtx, expiration time.Duration) error {
	store, err := p.sessionHolder.Get(ctx)

	if err != nil {
		return err
	}

	err = store.SetExpiration(expiration)

	if err != nil {
		return err
	}

	return p.sessionHolder.Save(ctx, store)
}

// GetExpiration get the expiration of the current session.
func (p *Provider) GetExpiration(ctx *fasthttp.RequestCtx) (time.Duration, error) {
	store, err := p.sessionHolder.Get(ctx)

	if err != nil {
		return time.Duration(0), err
	}

	return store.GetExpiration(), nil
}
