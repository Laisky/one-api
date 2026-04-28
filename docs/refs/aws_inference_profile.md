# AWS Bedrock Inference Profile

Refs:

- [aws adaptor](relay/adaptor/aws/adaptor.go)

## Requirements

This project is an aggregation website for AI APIs, supporting a variety of upstream AI API providers, referred to as Channel/Adaptors. After unified aggregation and transformation by this project, a unified OpenAI Completion API format is provided to users.

For the AWS adaptor, there is a new feature requirement: on the AWS channel edit page, add an input box named `inference_profile_arn_map`, where users can input a JSON in the format of `map[string]string`. This JSON provides a mapping that maps model names to AWS Bedrock Inference Profile ARN strings.

A new field also needs to be added to the Channel database table to store `inference_profile_arn_map`.

When users edit a channel, the `inference_profile_arn_map` needs to be validated:

1. The submitted JSON format must conform to `map[string]string`
2. Neither the key nor the value can be an empty string
3. `inference_profile_arn_map` can be empty

## Patch

Some implementation for this requirement has already been done previously, but the previous implementation was based on reading configuration files, not through the channel edit page and database.

There are also some other issues. Please, based on this edit, follow the requirements described above to correct previous mistakes and implement the required features.

FYI, here is the previous edits:

```patch
diff --git a/controller/channel-test.go b/controller/channel-test.go
index ae2960f2..ea3412b0 100644
--- a/controller/channel-test.go
+++ b/controller/channel-test.go
@@ -5,6 +5,7 @@ import (
 	"context"
 	"encoding/json"
 	"fmt"
+	claude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
 	"io"
 	"math"
 	"net/http"
@@ -131,8 +132,14 @@ func testChannel(ctx context.Context, channel *model.Channel, request *relaymode
 	if modelMap != nil && modelMap[modelName] != "" {
 		modelName = modelMap[modelName]
 	}
+	// aws bedrock sp Model meta.Config.AK,
+	arn := claude.FastClaudeModelTransArn(meta.Config.AK, modelName, meta.Config.Region)
 	meta.OriginModelName, meta.ActualModelName = request.Model, modelName
 	request.Model = modelName
+	// for aws tag arn
+	if arn != "" {
+		meta.ActualModelName = arn
+	}
 	convertedRequest, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
 	if err != nil {
 		return "", err, nil
diff --git a/docs/refs/aws_inference_profile.md b/docs/refs/aws_inference_profile.md
new file mode 100644
index 00000000..e69de29b
diff --git a/go.mod b/go.mod
index 02e58d8f..9ae80756 100644
--- a/go.mod
+++ b/go.mod
@@ -10,8 +10,9 @@ require (
 	github.com/Laisky/gin-middlewares/v7 v6.1.0
 	github.com/Laisky/go-utils/v6 v5.1.2-0.20250615144910-425272b31889
 	github.com/Laisky/zap v1.27.1-0.20241010063010-3154c45f2a1f
-	github.com/aws/aws-sdk-go-v2 v1.36.4
-	github.com/aws/aws-sdk-go-v2/credentials v1.17.69
+	github.com/aws/aws-sdk-go-v2 v1.36.5
+	github.com/aws/aws-sdk-go-v2/config v1.29.17
+	github.com/aws/aws-sdk-go-v2/credentials v1.17.70
 	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.30.1
 	github.com/coze-dev/coze-go v0.0.0-20250604025746-0d3b62f445d2
 	github.com/gin-contrib/cors v1.7.5
@@ -26,6 +27,7 @@ require (
 	github.com/gorilla/websocket v1.5.3
 	github.com/jinzhu/copier v0.4.0
 	github.com/joho/godotenv v1.5.1
+	github.com/json-iterator/go v1.1.12
 	github.com/patrickmn/go-cache v2.1.0+incompatible
 	github.com/pkoukk/tiktoken-go v0.1.7
 	github.com/prometheus/client_golang v1.22.0
@@ -55,9 +57,16 @@ require (
 	github.com/Laisky/graphql v1.0.6 // indirect
 	github.com/Laisky/pprof v0.0.0-20231102060718-a7a7fd2965ee // indirect
 	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
-	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.35 // indirect
-	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.35 // indirect
-	github.com/aws/smithy-go v1.22.2 // indirect
+	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
+	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
+	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
+	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
+	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
+	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
+	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
+	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
+	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
+	github.com/aws/smithy-go v1.22.4 // indirect
 	github.com/beorn7/perks v1.0.1 // indirect
 	github.com/bytedance/sonic v1.13.2 // indirect
 	github.com/bytedance/sonic/loader v0.2.4 // indirect
@@ -93,7 +102,6 @@ require (
 	github.com/jackc/puddle/v2 v2.2.2 // indirect
 	github.com/jinzhu/inflection v1.0.0 // indirect
 	github.com/jinzhu/now v1.1.5 // indirect
-	github.com/json-iterator/go v1.1.12 // indirect
 	github.com/jtolds/gls v4.20.0+incompatible // indirect
 	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
 	github.com/leodido/go-urn v1.4.0 // indirect
diff --git a/go.sum b/go.sum
index 3726f9ad..50fce7c9 100644
--- a/go.sum
+++ b/go.sum
@@ -74,20 +74,36 @@ github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751/go.mod h1:LOuy
 github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf/go.mod h1:ybxpYRFXyAe+OPACYpWeL0wqObRcbAqCMya13uyzqw0=
 github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4/go.mod h1:ybxpYRFXyAe+OPACYpWeL0wqObRcbAqCMya13uyzqw0=
 github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d/go.mod h1:rBZYJk541a8SKzHPHnH3zbiI+7dagKZ0cgpgrD7Fyho=
-github.com/aws/aws-sdk-go-v2 v1.36.4 h1:GySzjhVvx0ERP6eyfAbAuAXLtAda5TEy19E5q5W8I9E=
-github.com/aws/aws-sdk-go-v2 v1.36.4/go.mod h1:LLXuLpgzEbD766Z5ECcRmi8AzSwfZItDtmABVkRLGzg=
+github.com/aws/aws-sdk-go-v2 v1.36.5 h1:0OF9RiEMEdDdZEMqF9MRjevyxAQcf6gY+E7vwBILFj0=
+github.com/aws/aws-sdk-go-v2 v1.36.5/go.mod h1:EYrzvCCN9CMUTa5+6lf6MM4tq3Zjp8UhSGR/cBsjai0=
 github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 h1:zAybnyUQXIZ5mok5Jqwlf58/TFE7uvd3IAsa1aF9cXs=
 github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10/go.mod h1:qqvMj6gHLR/EXWZw4ZbqlPbQUyenf4h82UQUlKc+l14=
-github.com/aws/aws-sdk-go-v2/credentials v1.17.69 h1:8B8ZQboRc3uaIKjshve/XlvJ570R7BKNy3gftSbS178=
-github.com/aws/aws-sdk-go-v2/credentials v1.17.69/go.mod h1:gPME6I8grR1jCqBFEGthULiolzf/Sexq/Wy42ibKK9c=
-github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.35 h1:o1v1VFfPcDVlK3ll1L5xHsaQAFdNtZ5GXnNR7SwueC4=
-github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.35/go.mod h1:rZUQNYMNG+8uZxz9FOerQJ+FceCiodXvixpeRtdESrU=
-github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.35 h1:R5b82ubO2NntENm3SAm0ADME+H630HomNJdgv+yZ3xw=
-github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.35/go.mod h1:FuA+nmgMRfkzVKYDNEqQadvEMxtxl9+RLT9ribCwEMs=
+github.com/aws/aws-sdk-go-v2/config v1.29.17 h1:jSuiQ5jEe4SAMH6lLRMY9OVC+TqJLP5655pBGjmnjr0=
+github.com/aws/aws-sdk-go-v2/config v1.29.17/go.mod h1:9P4wwACpbeXs9Pm9w1QTh6BwWwJjwYvJ1iCt5QbCXh8=
+github.com/aws/aws-sdk-go-v2/credentials v1.17.70 h1:ONnH5CM16RTXRkS8Z1qg7/s2eDOhHhaXVd72mmyv4/0=
+github.com/aws/aws-sdk-go-v2/credentials v1.17.70/go.mod h1:M+lWhhmomVGgtuPOhO85u4pEa3SmssPTdcYpP/5J/xc=
+github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 h1:KAXP9JSHO1vKGCr5f4O6WmlVKLFFXgWYAGoJosorxzU=
+github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32/go.mod h1:h4Sg6FQdexC1yYG9RDnOvLbW1a/P986++/Y/a+GyEM8=
+github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 h1:SsytQyTMHMDPspp+spo7XwXTP44aJZZAC7fBV2C5+5s=
+github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36/go.mod h1:Q1lnJArKRXkenyog6+Y+zr7WDpk4e6XlR6gs20bbeNo=
+github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 h1:i2vNHQiXUvKhs3quBR6aqlgJaiaexz/aNvdCktW/kAM=
+github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36/go.mod h1:UdyGa7Q91id/sdyHPwth+043HhmP6yP9MBHgbZM0xo8=
+github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 h1:bIqFDwgGXXN1Kpp99pDOdKMTTb5d2KyU5X/BZxjOkRo=
+github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3/go.mod h1:H5O/EsxDWyU+LP/V8i5sm8cxoZgc2fdNR9bxlOFrQTo=
 github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.30.1 h1:TpBJYEk1dgZJgVqZ6ci+r3kbvB2oiZuDORiy0i4Ueag=
 github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.30.1/go.mod h1:LyIHS/IvMQGwxbLgrlb/sdxE+m0tZTuMDcqJeh0Pjh4=
-github.com/aws/smithy-go v1.22.2 h1:6D9hW43xKFrRx/tXXfAlIZc4JI+yQe6snnWcQyxSyLQ=
-github.com/aws/smithy-go v1.22.2/go.mod h1:irrKGvNn1InZwb2d7fkIRNucdfwR8R+Ts3wxYa/cJHg=
+github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 h1:CXV68E2dNqhuynZJPB80bhPQwAKqBWVer887figW6Jc=
+github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4/go.mod h1:/xFi9KtvBXP97ppCz1TAEvU1Uf66qvid89rbem3wCzQ=
+github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 h1:t0E6FzREdtCsiLIoLCWsYliNsRBgyGD/MCK571qk4MI=
+github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17/go.mod h1:ygpklyoaypuyDvOM5ujWGrYWpAK3h7ugnmKCU/76Ys4=
+github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 h1:AIRJ3lfb2w/1/8wOOSqYb9fUKGwQbtysJ2H1MofRUPg=
+github.com/aws/aws-sdk-go-v2/service/sso v1.25.5/go.mod h1:b7SiVprpU+iGazDUqvRSLf5XmCdn+JtT1on7uNL6Ipc=
+github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 h1:BpOxT3yhLwSJ77qIY3DoHAQjZsc4HEGfMCE4NGy3uFg=
+github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3/go.mod h1:vq/GQR1gOFLquZMSrxUK/cpvKCNVYibNyJ1m7JrU88E=
+github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 h1:NFOJ/NXEGV4Rq//71Hs1jC/NvPs1ezajK+yQmkwnPV0=
+github.com/aws/aws-sdk-go-v2/service/sts v1.34.0/go.mod h1:7ph2tGpfQvwzgistp2+zga9f+bCjlQJPkPUmMgDSD7w=
+github.com/aws/smithy-go v1.22.4 h1:uqXzVZNuNexwc/xrh6Tb56u89WDlJY6HS+KC0S4QSjw=
+github.com/aws/smithy-go v1.22.4/go.mod h1:t1ufH5HMublsJYulve2RKmHDC15xu1f26kHCp/HgceI=
 github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973/go.mod h1:Dwedo/Wpr24TaqPxmxbtue+5NUziq4I4S80YR8gNf3Q=
 github.com/beorn7/perks v1.0.0/go.mod h1:KWe93zE9D1o94FZ5RNwFwVgaQK1VOXiVxmqh+CedLV8=
 github.com/beorn7/perks v1.0.1 h1:VlbKKnNfV8bJzeqoa4cOKqO6bYr3WgKZxO8Z16+hsOM=
diff --git a/main.go b/main.go
index 4d739ca6..847f4da8 100644
--- a/main.go
+++ b/main.go
@@ -1,9 +1,11 @@
 package main

 import (
+	"context"
 	"embed"
 	"encoding/base64"
 	"fmt"
+	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
 	"net/http"
 	"os"
 	"runtime"
@@ -186,7 +188,12 @@ func main() {
 		server.GET("/metrics", middleware.AdminAuth(), gin.WrapH(promhttp.Handler()))
 		logger.SysLog("Prometheus metrics endpoint available at /metrics")
 	}
-
+	// special load
+	ctx := context.Background()
+	e := utils.LoadSpArn(ctx)
+	if e != nil {
+		logger.Warnf(ctx, "warn load special arn failed!")
+	}
 	router.SetRouter(server, buildFS)
 	var port = os.Getenv("PORT")
 	if port == "" {
diff --git a/relay/adaptor/anthropic/model.go b/relay/adaptor/anthropic/model.go
index f1b07714..f8891d79 100644
--- a/relay/adaptor/anthropic/model.go
+++ b/relay/adaptor/anthropic/model.go
@@ -59,7 +59,8 @@ type Request struct {
 	Tools         []Tool    `json:"tools,omitempty"`
 	ToolChoice    any       `json:"tool_choice,omitempty"`
 	//Metadata    `json:"metadata,omitempty"`
-	Thinking *model.Thinking `json:"thinking,omitempty"`
+	Thinking         *model.Thinking `json:"thinking,omitempty"`
+	AnthropicVersion string          `json:"anthropic_version,omitempty"`
 }

 type Usage struct {
diff --git a/relay/adaptor/aws/adaptor.go b/relay/adaptor/aws/adaptor.go
index e2e093d1..0bda9f0a 100644
--- a/relay/adaptor/aws/adaptor.go
+++ b/relay/adaptor/aws/adaptor.go
@@ -1,11 +1,10 @@
 package aws

 import (
-	"io"
-	"net/http"
-
+	"context"
 	"github.com/Laisky/errors/v2"
 	"github.com/aws/aws-sdk-go-v2/aws"
+	"github.com/aws/aws-sdk-go-v2/config"
 	"github.com/aws/aws-sdk-go-v2/credentials"
 	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
 	"github.com/gin-gonic/gin"
@@ -13,23 +12,30 @@ import (
 	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
 	"github.com/Laisky/one-api/relay/meta"
 	"github.com/Laisky/one-api/relay/model"
+	"io"
+	"net/http"
 )

 var _ adaptor.Adaptor = new(Adaptor)

 type Adaptor struct {
 	awsAdapter utils.AwsAdapter
-
-	Meta      *meta.Meta
-	AwsClient *bedrockruntime.Client
+	Config     aws.Config
+	Meta       *meta.Meta
+	AwsClient  *bedrockruntime.Client
 }

 func (a *Adaptor) Init(meta *meta.Meta) {
 	a.Meta = meta
-	a.AwsClient = bedrockruntime.New(bedrockruntime.Options{
-		Region:      meta.Config.Region,
-		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(meta.Config.AK, meta.Config.SK, "")),
-	})
+	defaultConfig, err := config.LoadDefaultConfig(context.Background(),
+		config.WithRegion(meta.Config.Region),
+		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
+			meta.Config.AK, meta.Config.SK, "")))
+	if err != nil {
+		return
+	}
+	a.Config = defaultConfig
+	a.AwsClient = bedrockruntime.NewFromConfig(defaultConfig)
 }

 func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
diff --git a/relay/adaptor/aws/claude/adapter.go b/relay/adaptor/aws/claude/adapter.go
index 1aa00fd1..df784cd5 100644
--- a/relay/adaptor/aws/claude/adapter.go
+++ b/relay/adaptor/aws/claude/adapter.go
@@ -25,7 +25,10 @@ func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.G
 	if err != nil {
 		return nil, errors.Wrap(err, "convert request")
 	}
-
+	claudeReq.AnthropicVersion = "bedrock-2023-05-31"
+	claudeReq.MaxTokens = request.MaxTokens
+	claudeReq.TopK = request.TopK
+	claudeReq.TopP = request.TopP
 	c.Set(ctxkey.RequestModel, request.Model)
 	c.Set(ctxkey.ConvertedRequest, claudeReq)
 	return claudeReq, nil
diff --git a/relay/adaptor/aws/claude/main.go b/relay/adaptor/aws/claude/main.go
index e850cbd4..709d8a88 100644
--- a/relay/adaptor/aws/claude/main.go
+++ b/relay/adaptor/aws/claude/main.go
@@ -27,34 +27,56 @@ import (

 // https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
 var AwsModelIDMap = map[string]string{
-	"claude-instant-1.2":         "anthropic.claude-instant-v1",
-	"claude-2.0":                 "anthropic.claude-v2",
-	"claude-2.1":                 "anthropic.claude-v2:1",
-	"claude-3-haiku-20240307":    "anthropic.claude-3-haiku-20240307-v1:0",
-	"claude-3-sonnet-20240229":   "anthropic.claude-3-sonnet-20240229-v1:0",
-	"claude-3-opus-20240229":     "anthropic.claude-3-opus-20240229-v1:0",
-	"claude-opus-4-20250514":     "anthropic.claude-opus-4-20250514-v1:0",
-	"claude-3-5-sonnet-20240620": "anthropic.claude-3-5-sonnet-20240620-v1:0",
-	"claude-3-5-sonnet-20241022": "anthropic.claude-3-5-sonnet-20241022-v2:0",
-	"claude-3-5-sonnet-latest":   "anthropic.claude-3-5-sonnet-20241022-v2:0",
-	"claude-3-5-haiku-20241022":  "anthropic.claude-3-5-haiku-20241022-v1:0",
-	"claude-3-7-sonnet-latest":   "anthropic.claude-3-7-sonnet-20250219-v1:0",
-	"claude-3-7-sonnet-20250219": "anthropic.claude-3-7-sonnet-20250219-v1:0",
-	"claude-sonnet-4-20250514":   "anthropic.claude-sonnet-4-20250514-v1:0",
+	"claude-instant-1.2":           "anthropic.claude-instant-v1",
+	"claude-2.0":                   "anthropic.claude-v2",
+	"claude-2.1":                   "anthropic.claude-v2:1",
+	"claude-3-haiku-20240307":      "anthropic.claude-3-haiku-20240307-v1:0",
+	"claude-3-sonnet-20240229":     "anthropic.claude-3-sonnet-20240229-v1:0",
+	"claude-3-opus-20240229":       "anthropic.claude-3-opus-20240229-v1:0",
+	"claude-opus-4-20250514":       "anthropic.claude-opus-4-20250514-v1:0",
+	"claude-3-5-sonnet-20240620":   "anthropic.claude-3-5-sonnet-20240620-v1:0",
+	"claude-3-5-sonnet-20241022":   "anthropic.claude-3-5-sonnet-20241022-v2:0",
+	"claude-3-5-sonnet-latest":     "anthropic.claude-3-5-sonnet-20241022-v2:0",
+	"claude-3-5-haiku-20241022":    "anthropic.claude-3-5-haiku-20241022-v1:0",
+	"claude-3-7-sonnet-latest":     "anthropic.claude-3-7-sonnet-20250219-v1:0",
+	"claude-3-7-sonnet-20250219":   "anthropic.claude-3-7-sonnet-20250219-v1:0",
+	"claude-sonnet-4-20250514":     "anthropic.claude-sonnet-4-20250514-v1:0",
+	"claude-3-7-sonnet-latest-tag": "claude-3-7-sonnet-latest-tag",
+	"claude-4-sonnet-latest-tag":   "claude-4-sonnet-latest-tag",
 }

-func awsModelID(requestModel string) (string, error) {
+func AwsModelID(requestModel string) (string, error) {
 	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
 		return awsModelID, nil
 	}
-
 	return "", errors.Errorf("model %s not found", requestModel)
 }

+func AwsClaudeModelTransArn(c *gin.Context, awsCli *bedrockruntime.Client) string {
+	reqModelID := c.GetString(ctxkey.RequestModel)
+	arn := ""
+	ak := ""
+	cred, err := awsCli.Options().Credentials.Retrieve(c)
+	if err != nil {
+		logger.Warnf(c, "%v", err)
+	} else {
+		ak = cred.AccessKeyID
+	}
+	arn = FastClaudeModelTransArn(ak, reqModelID, awsCli.Options().Region)
+	return arn
+}
+
+func FastClaudeModelTransArn(ak, model, region string) (arn string) {
+	if model == "claude-3-7-sonnet-latest-tag" || model == "claude-4-sonnet-latest-tag" {
+		arn = utils.FastAwsArn(ak, model, region)
+	}
+	return arn
+}
+
 func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
-	awsModelID, err := awsModelID(c.GetString(ctxkey.RequestModel))
+	awsModelID, err := AwsModelID(c.GetString(ctxkey.RequestModel))
 	if err != nil {
-		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
+		return utils.WrapErr(errors.Wrap(err, "AwsModelID")), nil
 	}

 	// Use the enhanced cross-region profile conversion with fallback testing
@@ -65,6 +87,11 @@ func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*
 		ContentType: aws.String("application/json"),
 	}

+	if arn := AwsClaudeModelTransArn(c, awsCli); arn != "" {
+		awsReq.ModelId = aws.String(arn)
+		logger.Debugf(c, "final use modelID [%s]", arn)
+	}
+
 	claudeReq_, ok := c.Get(ctxkey.ConvertedRequest)
 	if !ok {
 		return utils.WrapErr(errors.New("request not found")), nil
@@ -115,9 +142,9 @@ func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*

 func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
 	createdTime := helper.GetTimestamp()
-	awsModelID, err := awsModelID(c.GetString(ctxkey.RequestModel))
+	awsModelID, err := AwsModelID(c.GetString(ctxkey.RequestModel))
 	if err != nil {
-		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
+		return utils.WrapErr(errors.Wrap(err, "AwsModelID")), nil
 	}

 	// Use the enhanced cross-region profile conversion with fallback testing
@@ -128,6 +155,11 @@ func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.E
 		ContentType: aws.String("application/json"),
 	}

+	if arn := AwsClaudeModelTransArn(c, awsCli); arn != "" {
+		awsReq.ModelId = aws.String(arn)
+		logger.Debugf(c, "final use modelID [%s]", arn)
+	}
+
 	claudeReq_, ok := c.Get(ctxkey.ConvertedRequest)
 	if !ok {
 		return utils.WrapErr(errors.New("request not found")), nil
diff --git a/relay/adaptor/aws/llama3/main.go b/relay/adaptor/aws/llama3/main.go
index dc11741b..a680ff41 100644
--- a/relay/adaptor/aws/llama3/main.go
+++ b/relay/adaptor/aws/llama3/main.go
@@ -74,7 +74,6 @@ func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*
 	if err != nil {
 		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
 	}
-
 	awsModelID = utils.ConvertModelID2CrossRegionProfile(awsModelID, awsCli.Options().Region)
 	awsReq := &bedrockruntime.InvokeModelInput{
 		ModelId:     aws.String(awsModelID),
@@ -145,7 +144,6 @@ func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.E
 	if err != nil {
 		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
 	}
-
 	awsModelID = utils.ConvertModelID2CrossRegionProfile(awsModelID, awsCli.Options().Region)
 	awsReq := &bedrockruntime.InvokeModelWithResponseStreamInput{
 		ModelId:     aws.String(awsModelID),
diff --git a/relay/adaptor/aws/registry.go b/relay/adaptor/aws/registry.go
index 5f655480..6be027d1 100644
--- a/relay/adaptor/aws/registry.go
+++ b/relay/adaptor/aws/registry.go
@@ -1,9 +1,12 @@
 package aws

 import (
+	"context"
+	"github.com/Laisky/one-api/common/logger"
 	claude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
 	llama3 "github.com/Laisky/one-api/relay/adaptor/aws/llama3"
 	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
+	"regexp"
 )

 type AwsModelType int
@@ -16,6 +19,7 @@ const (
 var (
 	adaptors = map[string]AwsModelType{}
 )
+var awsArnMatch *regexp.Regexp

 func init() {
 	for model := range claude.AwsModelIDMap {
@@ -24,10 +28,19 @@ func init() {
 	for model := range llama3.AwsModelIDMap {
 		adaptors[model] = AwsLlama3
 	}
+	match, err := regexp.Compile("arn:aws:bedrock.+claude")
+	if err != nil {
+		logger.Warnf(context.Background(), "compile %v", err)
+		return
+	}
+	awsArnMatch = match
 }

 func GetAdaptor(model string) utils.AwsAdapter {
 	adaptorType := adaptors[model]
+	if awsArnMatch.MatchString(model) {
+		adaptorType = AwsClaude
+	}
 	switch adaptorType {
 	case AwsClaude:
 		return &claude.Adaptor{}
diff --git a/relay/adaptor/aws/utils/consts.go b/relay/adaptor/aws/utils/consts.go
index ad393e0d..d08a8622 100644
--- a/relay/adaptor/aws/utils/consts.go
+++ b/relay/adaptor/aws/utils/consts.go
@@ -2,6 +2,8 @@ package utils

 import (
 	"context"
+	jsoniter "github.com/json-iterator/go"
+	"os"
 	"slices"
 	"strings"
 	"sync"
@@ -264,9 +266,67 @@ func testModelAvailability(ctx context.Context, client *bedrockruntime.Client, m
 	return true
 }

+var arnMap = map[string]string{}
+
+// Config 定义配置结构
+
+var lock = &sync.RWMutex{}
+
+type Config struct {
+	AK     string `json:"ak"`
+	Model  string `json:"model"`
+	Region string `json:"region"`
+	ARN    string `json:"arn"`
+}
+
+func LoadSpArn(ctx context.Context) error {
+	// 读取JSON文件
+	data, err := os.ReadFile("./conf/arn.json")
+	if err != nil {
+		logger.Warnf(ctx, "读取文件失败: %v", err)
+		return err
+	}
+	var configs []Config
+	err = jsoniter.Unmarshal(data, &configs)
+	if err != nil {
+		logger.Warnf(ctx, "解析JSON失败: %v", err)
+		return err
+	}
+	lock.Lock()
+	defer lock.Unlock()
+	arnMap = map[string]string{}
+	// 输出解析结果
+	for _, config := range configs {
+		logger.Infof(ctx, "load special keymap AK[%s], model[%s], region[%s], arn[%s]", config.AK, config.Model, config.Region, config.ARN)
+		key := KeyCombine(config.AK, config.Model, config.Region)
+		arnMap[key] = config.ARN
+		// 输出解析结果
+	}
+	// 解析JSON数据
+	return nil
+}
+
+func KeyCombine(ak, model, region string) string {
+	combineName := strings.Join([]string{ak, model, region}, "::")
+	return combineName
+}
+
+func FastAwsArn(ak, model, region string) (arn string) {
+	lock.RLock()
+	defer lock.RUnlock()
+	key := KeyCombine(ak, model, region)
+	logger.Debugf(context.Background(), "combineKey[%s]", key)
+	arn, ok := arnMap[key]
+	if !ok {
+		return ""
+	}
+	return arn
+}
+
 // ConvertModelID2CrossRegionProfile converts the model ID to a cross-region profile ID.
 // Enhanced version that uses aws-sdk-go-v2 patterns and includes availability testing.
 func ConvertModelID2CrossRegionProfile(model, region string) string {
+
 	regionPrefix := getRegionPrefix(region)
 	if regionPrefix == "" {
 		logger.Debugf(context.TODO(), "unsupported region for cross-region inference: %s", region)
diff --git a/relay/model/general.go b/relay/model/general.go
index 7e17d982..5a15bf57 100644
--- a/relay/model/general.go
+++ b/relay/model/general.go
@@ -50,6 +50,7 @@ type GeneralOpenAIRequest struct {
 	// https://platform.openai.com/docs/api-reference/chat/create
 	Messages []Message `json:"messages,omitempty"`
 	Model    string    `json:"model,omitempty"`
+	Arn      string    `json:"arn,omitempty"` // for aws arn
 	Store    *bool     `json:"store,omitempty"`
 	Metadata any       `json:"metadata,omitempty"`
 	// FrequencyPenalty is a number between -2.0 and 2.0 that penalizes
```
