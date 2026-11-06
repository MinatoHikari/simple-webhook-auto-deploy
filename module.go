package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// Hook webhook 认证头。
type Hook struct {
	GitlabToken     string `header:"X-Gitlab-Token"`
	GithubSignature string `header:"X-Hub-Signature-256"`
}

// RequestBody webhook 请求体(gitlab / github / 自定义触发)。
type RequestBody struct {
	Ref    string `json:"ref"`
	Branch string `json:"branch"`
}

// Verify 验证 webhook 请求权限:
// GitLab 为 token 明文比对(常数时间),GitHub 为 HMAC-SHA256 签名校验。
func Verify(hook Hook, token string, reqBody []byte) bool {
	if token == "" {
		// 未配置任何 token 时拒绝所有 webhook,避免裸奔
		return false
	}
	if hook.GitlabToken != "" {
		return subtle.ConstantTimeCompare([]byte(hook.GitlabToken), []byte(token)) == 1
	}
	if hook.GithubSignature != "" {
		signature := strings.TrimPrefix(hook.GithubSignature, "sha256=")
		signatureByte, err := hex.DecodeString(signature)
		if err != nil || len(signatureByte) == 0 {
			return false
		}
		mac := hmac.New(sha256.New, []byte(token))
		mac.Write(reqBody)
		return hmac.Equal(mac.Sum(nil), signatureByte)
	}
	return false
}

// BranchMatches 分支过滤:返回请求中解析出的分支名与是否放行。
// 配置 branch 为空时不过滤(兼容无分支信息的自定义触发)。
func BranchMatches(cfgBranch string, body RequestBody) (string, bool) {
	branch := body.Branch
	if branch == "" && body.Ref != "" {
		parts := strings.Split(body.Ref, "/")
		if len(parts) >= 3 {
			branch = parts[len(parts)-1]
		}
	}
	if cfgBranch == "" || branch == "" {
		return branch, true
	}
	return branch, branch == cfgBranch
}
