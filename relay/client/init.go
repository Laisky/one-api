package client

// import (
// 	"net/http"
// 	"os"
// 	"time"

// 	gutils "github.com/Laisky/go-utils/v6"
// 	"github.com/Laisky/one-api/common/config"
// )

// var HTTPClient *http.Client
// var ImpatientHTTPClient *http.Client

// func init() {
// 	var opts []gutils.HTTPClientOptFunc

// 	timeout := time.Duration(max(config.IdleTimeout, 30)) * time.Second
// 	opts = append(opts, gutils.WithHTTPClientTimeout(timeout))
// 	if os.Getenv("RELAY_PROXY") != "" {
// 		opts = append(opts, gutils.WithHTTPClientProxy(os.Getenv("RELAY_PROXY")))
// 	}

// 	var err error
// 	HTTPClient, err = gutils.NewHTTPClient(opts...)
// 	if err != nil {
// 		panic(err)
// 	}

// 	ImpatientHTTPClient, err = gutils.NewHTTPClient(
// 		gutils.WithHTTPClientTimeout(5 * time.Second),
// 	)
// 	if err != nil {
// 		panic(err)
// 	}
// }
