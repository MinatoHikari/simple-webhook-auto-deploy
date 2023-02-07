package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/kataras/golog"
	"strings"
)

// CheckBranch 分支过滤
func CheckBranch(branch string, reqBodyMap RequestBody, logger *golog.Logger) bool {
	if &reqBodyMap.Branch != nil {
		return reqBodyMap.Branch == branch
	}

	refArr := strings.Split(reqBodyMap.Ref, "/")
	if refArr[2] != branch {
		logger.Info("branch checked failed, stop deploy...")

		return false
	}

	return true
}

// Verify 验证请求权限
func Verify(hook Hook, token string, reqBody []byte) bool {
	if hook.GitlabToken != "" {
		return hook.GitlabToken == token
	}
	if hook.GithubSignature != "" {
		signToken := hmac.New(sha256.New, []byte(token))
		signToken.Write(reqBody)
		hexstr := hex.EncodeToString(signToken.Sum(nil))
		signature := strings.Split(hook.GithubSignature, "sha256=")[1]
		signatureByte, err := hex.DecodeString(signature)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(hexstr)
		fmt.Println(signature)
		return hmac.Equal(signToken.Sum(nil), signatureByte)
	}
	return false
}
