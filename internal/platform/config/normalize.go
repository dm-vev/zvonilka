package config

import "strings"

func (c *Configuration) normalize() {
	c.Service.Name = strings.TrimSpace(c.Service.Name)
	c.Service.Environment = strings.ToLower(strings.TrimSpace(c.Service.Environment))
	if c.Service.Environment == "" {
		c.Service.Environment = defaultEnvironment
	}

	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))
	if c.Logging.Level == "" {
		c.Logging.Level = defaultLogging(c.Service.Environment).Level
	}
	c.Logging.Format = strings.ToLower(strings.TrimSpace(c.Logging.Format))
	if c.Logging.Format == "" {
		c.Logging.Format = defaultLogging(c.Service.Environment).Format
	}

	c.Runtime.HTTP.Address = strings.TrimSpace(c.Runtime.HTTP.Address)
	c.Runtime.GRPC.Address = strings.TrimSpace(c.Runtime.GRPC.Address)

	c.Infrastructure.Postgres.DSN = strings.TrimSpace(c.Infrastructure.Postgres.DSN)
	c.Infrastructure.Postgres.MigrationsPath = strings.TrimSpace(c.Infrastructure.Postgres.MigrationsPath)
	c.Infrastructure.Postgres.Schema = strings.TrimSpace(c.Infrastructure.Postgres.Schema)

	c.Infrastructure.Redis.Address = strings.TrimSpace(c.Infrastructure.Redis.Address)
	c.Infrastructure.Redis.Username = strings.TrimSpace(c.Infrastructure.Redis.Username)
	c.Infrastructure.Redis.Password = strings.TrimSpace(c.Infrastructure.Redis.Password)

	c.Infrastructure.ObjectStore.Endpoint = strings.TrimSpace(c.Infrastructure.ObjectStore.Endpoint)
	c.Infrastructure.ObjectStore.Region = strings.TrimSpace(c.Infrastructure.ObjectStore.Region)
	c.Infrastructure.ObjectStore.Bucket = strings.TrimSpace(c.Infrastructure.ObjectStore.Bucket)
	c.Infrastructure.ObjectStore.AccessKeyID = strings.TrimSpace(c.Infrastructure.ObjectStore.AccessKeyID)
	c.Infrastructure.ObjectStore.SecretAccessKey = strings.TrimSpace(c.Infrastructure.ObjectStore.SecretAccessKey)

	c.Infrastructure.Telemetry.OTLPAddress = strings.TrimSpace(c.Infrastructure.Telemetry.OTLPAddress)
	c.Infrastructure.Telemetry.SentryDSN = strings.TrimSpace(c.Infrastructure.Telemetry.SentryDSN)
}
