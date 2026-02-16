package config

type ApiConfig struct {
	Database struct {
		Host            string `toml:"host" env:"BTC_GIFTCARD_DB_HOST"`
		Port            string `toml:"port" env:"BTC_GIFTCARD_DB_PORT" env-default:"5432"`
		User            string `toml:"user" env:"BTC_GIFTCARD_DB_USER"`
		Password        string `toml:"password" env:"BTC_GIFTCARD_DB_PASSWORD"`
		DB              string `toml:"db" env:"BTC_GIFTCARD_DB_NAME"`
		SslMode         string `toml:"ssl_mode" env:"BTC_GIFTCARD_DB_SSL_MODE" env-default:"disable"`
		MaxConns        int    `toml:"max_conns" env:"BTC_GIFTCARD_DB_MAX_CONNS" env-default:"25"`
		MinConns        int    `toml:"min_conns" env:"BTC_GIFTCARD_DB_MIN_CONNS" env-default:"5"`
		MaxConnLifetime int    `toml:"max_conn_lifetime" env:"BTC_GIFTCARD_DB_MAX_CONN_LIFETIME" env-default:"5"`
		MaxConnIdleTime int    `toml:"max_conn_idle_time" env:"BTC_GIFTCARD_DB_MAX_CONN_IDLE_TIME" env-default:"1"`
	} `toml:"database"`

	Redis struct {
		Host     string `toml:"host" env:"BTC_GIFTCARD_REDIS_HOST"`
		Port     string `toml:"port" env:"BTC_GIFTCARD_REDIS_PORT" env-default:"6379"`
		Password string `toml:"password" env:"BTC_GIFTCARD_REDIS_PASSWORD"`
		DB       int    `toml:"db" env:"BTC_GIFTCARD_REDIS_DB" env-default:"0"`
	} `toml:"redis"`
}
