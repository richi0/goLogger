# goLogger [![GoDoc](https://pkg.go.dev/badge/goLogger.svg)](https://pkg.go.dev/github.com/richi0/goLogger)

goLogger sends logs to external sources

## What does goLogger do?

goLogger sends logs to external sources. The only supportet source at the moment is NewRelic.

## How do I use goLogger?

### Install

```
go get -u github.com/richi0/goLogger
```

### Usage Example

```go
// initializes a new logger that sends logs to newRelic on prod
writer := os.Stdout
logger := goLogger.New(writer)
if config.Stage == "prod" {
	logger = goLogger.New(writer,
		goLogger.NewNewRelicLogger(
			config.NewRelicEndpoint,
			config.NewRelicLicenseKey,
		),
	)
}
```

## Documentation

Find the full documentation of the package here: https://pkg.go.dev/github.com/richi0/goLogger
