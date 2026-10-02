package config

const (
	// defaultSQLMaxIdleConns keeps enough warm connections for a single
	// instance serving about 100 requests per second without retaining a large
	// number of idle database backends.
	defaultSQLMaxIdleConns = 10

	// defaultSQLMaxOpenConns allows bursts above the expected database
	// concurrency while leaving capacity for administration and other services.
	defaultSQLMaxOpenConns = 50

	// defaultSQLMaxLifetimeSeconds periodically recycles connections so stale
	// sessions do not remain in the pool indefinitely.
	defaultSQLMaxLifetimeSeconds = 300
)
