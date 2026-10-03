package xunfei

import (
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
)

// https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E

//Spark4.0 Ultra request address, corresponding domain parameter is 4.0Ultra:
//
//wss://spark-api.xf-yun.com/v4.0/chat
//Spark Max-32K request address, corresponding domain parameter is max-32k
//
//wss://spark-api.xf-yun.com/chat/max-32k
//Spark Max request address, corresponding domain parameter is generalv3.5
//
//wss://spark-api.xf-yun.com/v3.5/chat
//Spark Pro-128K请求地址，对应的domain参数为pro-128k：
//
// wss://spark-api.xf-yun.com/chat/pro-128k
//Spark Pro请求地址，对应的domain参数为generalv3：
//
//wss://spark-api.xf-yun.com/v3.1/chat
//Spark Lite请求地址，对应的domain参数为lite：
//
//wss://spark-api.xf-yun.com/v1.1/chat

// Lite、Pro、Pro-128K、Max、Max-32K和4.0 Ultra

func parseAPIVersionByModelName(modelName string) string {
	apiVersion := modelName2APIVersion(modelName)
	if apiVersion != "" {
		return apiVersion
	}

	index := strings.IndexAny(modelName, "-")
	if index != -1 {
		return modelName[index+1:]
	}
	return ""
}

func modelName2APIVersion(modelName string) string {
	switch modelName {
	case "Spark-Lite":
		return "v1.1"
	case "Spark-Pro":
		return "v3.1"
	case "Spark-Pro-128K":
		return "v3.1-128K"
	case "Spark-Max":
		return "v3.5"
	case "Spark-Max-32K":
		return "v3.5-32K"
	case "Spark-4.0-Ultra":
		return "v4.0"
	}
	return ""
}

// https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
func apiVersion2domain(apiVersion string) string {
	switch apiVersion {
	case "v1.1":
		return "lite"
	case "v2.1":
		return "generalv2"
	case "v3.1":
		return "generalv3"
	case "v3.1-128K":
		return "pro-128k"
	case "v3.5":
		return "generalv3.5"
	case "v3.5-32K":
		return "max-32k"
	case "v4.0":
		return "4.0Ultra"
	}
	return "general" + apiVersion
}

// getXunfeiAuthUrl builds the Xunfei websocket authorization URL for an API
// version. Parameters: apiVersion selects the Xunfei endpoint, apiKey and
// apiSecret authenticate the request. Returns: the provider domain, signed URL,
// and a wrapped error when URL construction fails.
func getXunfeiAuthUrl(apiVersion string, apiKey string, apiSecret string) (string, string, error) {
	var authUrl string
	domain := apiVersion2domain(apiVersion)
	var err error
	switch apiVersion {
	case "v3.1-128K":
		authUrl, err = buildXunfeiAuthUrl("wss://spark-api.xf-yun.com/chat/pro-128k", apiKey, apiSecret)
	case "v3.5-32K":
		authUrl, err = buildXunfeiAuthUrl("wss://spark-api.xf-yun.com/chat/max-32k", apiKey, apiSecret)
	default:
		authUrl, err = buildXunfeiAuthUrl(fmt.Sprintf("wss://spark-api.xf-yun.com/%s/chat", apiVersion), apiKey, apiSecret)
	}
	if err != nil {
		return "", "", errors.Wrap(err, "build Xunfei authorization URL")
	}
	return domain, authUrl, nil
}
