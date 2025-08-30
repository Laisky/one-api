package aws

import (
	"fmt"
	"regexp"

	"github.com/songquanpeng/one-api/common/logger"
	claude "github.com/songquanpeng/one-api/relay/adaptor/aws/claude"
	llama3 "github.com/songquanpeng/one-api/relay/adaptor/aws/llama3"
	mistral "github.com/songquanpeng/one-api/relay/adaptor/aws/mistral"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
)

type AwsModelType int

const (
	AwsClaude AwsModelType = iota + 1
	AwsLlama3
	AwsMistral
)

var (
	adaptors = map[string]AwsModelType{}
)
var awsArnMatch *regexp.Regexp

func init() {
	for model := range claude.AwsModelIDMap {
		adaptors[model] = AwsClaude
	}
	for model := range llama3.AwsModelIDMap {
		adaptors[model] = AwsLlama3
	}
	for model := range mistral.AwsModelIDMap {
		adaptors[model] = AwsMistral
	}
	match, err := regexp.Compile("arn:aws:bedrock.+claude")
	if err != nil {
		logger.Logger.Warn(fmt.Sprintf("compile %v", err))
		return
	}
	awsArnMatch = match
}

func GetAdaptor(model string) utils.AwsAdapter {
	adaptorType := adaptors[model]
	if awsArnMatch.MatchString(model) {
		adaptorType = AwsClaude
	}
	switch adaptorType {
	case AwsClaude:
		return &claude.Adaptor{}
	case AwsLlama3:
		return &llama3.Adaptor{}
	case AwsMistral:
		return &mistral.Adaptor{}
	default:
		return nil
	}
}
