package config

import (
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"os"
	"path/filepath"
)

type AllOptions struct {
	Bind  string `name:"bind" usage:"Bind address the server will listen on" env:"BIND"`
	Debug *bool  `name:"debug" usage:"Enables debug logging" env:"DEBUG"`

	NatsURL                 *string `name:"nats-url" usage:"URL for a NATS server (when using NATS for pubsub)" env:"NATS_URL" category:"NATS"`
	NatsSubscriptionSubject *string `name:"nats-subscription-subject" usage:"Name of a NATS subscription subject where data will be exchanged" env:"NATS_SUBSCRIPTION_SUBJECT" category:"NATS" value:"udpfw-dispatch-exchange"`

	NatsUserCredentials     *FilePath `name:"nats-user-credentials" usage:"NATS user's JWT path" env:"NATS_USER_CREDENTIALS_PATH" category:"NATS Authentication"`
	NatsUserCredentialsNKey *FilePath `name:"nats-user-credentials-nkey" usage:"NATS user's private Nkey seed path" env:"NATS_USER_CREDENTIALS_NKEY_PATH" category:"NATS Authentication"`
	NatsNkeySeed            *FilePath `name:"nats-nkey-from-seed" usage:"NATS user's bare nkey seed path" env:"NATS_NKEY_SEED_PATH" category:"NATS Authentication"`
	NatsRootCA              *FilePath `name:"nats-root-ca" usage:"Path to Root CA for a self-signed TLS certificate" env:"NATS_ROOT_CA_PATH" category:"NATS TLS" `
	NatsClientCertificate   *FilePath `name:"nats-client-certificate" usage:"NATS client certificate path" env:"NATS_CLIENT_CERTIFICATE_PATH" category:"NATS TLS" `
	NatsClientKey           *FilePath `name:"nats-client-key" usage:"NATS client certificate key path" env:"NATS_CLIENT_CERTIFICATE_KEY_PATH" category:"NATS TLS" `

	RedisURL           *string `name:"redis-url" usage:"Redis URL (when using Redis for pubsub)" env:"REDIS_PATH" category:"Redis" `
	RedisPubsubChannel *string `name:"redis-pubsub-channel" usage:"Redis channel name where data will be exchanged" env:"REDIS_PUBSUB_CHANNEL" category:"Redis" value:"udpfw-dispatch-exchange"`
}

type FilePath string

func (f FilePath) Clean() (string, error) {
	path, err := filepath.Abs(string(f))
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if stat.IsDir() {
		return "", fmt.Errorf("%s: is a directory", path)
	}

	return path, nil
}

type Context struct {
	BindAddress   string
	PubSubService any // *NATSConfig, *RedisConfig, or nil
	Debug         bool
}

type NATSConfig struct {
	URL               string
	Subject           string
	ConnectionOptions []nats.Option
}

type RedisConfig struct {
	Options *redis.Options
	Channel string
}

func (a *AllOptions) IntoContext() (*Context, error) {
	if a.NatsURL != nil && a.RedisURL != nil {
		return nil, fmt.Errorf("define either --nats-url or --redis-url, not both")
	}

	ctx := Context{
		BindAddress: a.Bind,
		Debug:       a.Debug != nil && *a.Debug,
	}

	if a.NatsURL != nil {
		if a.NatsUserCredentials == nil && a.NatsUserCredentialsNKey != nil {
			return nil, fmt.Errorf("--nats-user-credentials-nkey must be used with --nats-user-credentials")
		}

		if (a.NatsClientKey != nil && a.NatsClientCertificate == nil) ||
			(a.NatsClientKey == nil && a.NatsClientCertificate != nil) {
			return nil, fmt.Errorf("--nats-client-certificate and --nats-client-key must be both present or absent")
		}

		natsConfig := &NATSConfig{
			URL:     *a.NatsURL,
			Subject: *a.NatsSubscriptionSubject,
		}

		if a.NatsUserCredentials != nil {
			credsPath, err := a.NatsUserCredentials.Clean()
			if err != nil {
				return nil, err
			}

			if a.NatsUserCredentialsNKey != nil {
				nkeyPath, err := a.NatsUserCredentialsNKey.Clean()
				if err != nil {
					return nil, err
				}
				natsConfig.ConnectionOptions = append(natsConfig.ConnectionOptions,
					nats.UserCredentials(credsPath, nkeyPath))
			} else {
				natsConfig.ConnectionOptions = append(natsConfig.ConnectionOptions,
					nats.UserCredentials(credsPath))
			}
		}

		if a.NatsNkeySeed != nil {
			nkeyPath, err := a.NatsNkeySeed.Clean()
			if err != nil {
				return nil, err
			}
			opt, err := nats.NkeyOptionFromSeed(nkeyPath)
			if err != nil {
				return nil, err
			}
			natsConfig.ConnectionOptions = append(natsConfig.ConnectionOptions, opt)
		}

		if a.NatsRootCA != nil {
			rootPath, err := a.NatsRootCA.Clean()
			if err != nil {
				return nil, err
			}
			natsConfig.ConnectionOptions = append(natsConfig.ConnectionOptions,
				nats.RootCAs(rootPath))
		}

		if a.NatsClientCertificate != nil {
			clientCertPath, err := a.NatsClientCertificate.Clean()
			if err != nil {
				return nil, err
			}

			clientKeyPath, err := a.NatsClientKey.Clean()
			if err != nil {
				return nil, err
			}

			natsConfig.ConnectionOptions = append(natsConfig.ConnectionOptions,
				nats.ClientCert(clientCertPath, clientKeyPath))
		}

		ctx.PubSubService = natsConfig
	}

	if a.RedisURL != nil {
		opts, err := redis.ParseURL(*a.RedisURL)
		if err != nil {
			return nil, err
		}

		ctx.PubSubService = &RedisConfig{
			Options: opts,
			Channel: *a.RedisPubsubChannel,
		}
	}

	return &ctx, nil
}
