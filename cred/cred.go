package mycred

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/larksuite/cli/extension/credential"
)

const (
    appID     = "managed-by-ipass" // 替换为真实 AppID
    appSecret = "XXXXXXXXXXXXXXXXXXXXXXXX" // 替换为真实 AppSecret
)

type Provider struct{}

func (p *Provider) Name() string { return "mycred" }

// ResolveAccount 返回应用凭证
// 返回 &Account{} → 命中，CLI 用这组凭证
// 返回 nil, nil   → 跳过，交给下一个 Provider
// 返回 nil, err   → 报错，终止
func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
    // 这里替换成你自己的逻辑：查数据库、调 Vault、读配置中心...
    return &credential.Account{
        AppID:     appID,
        AppSecret: appSecret,
        Brand:     credential.BrandFeishu,
        DefaultAs: credential.IdentityUser,
    }, nil
}

// ResolveToken 返回访问令牌
// 重要：一旦你的 Provider 接管了 Account，Token 也由你负责。
// 返回 nil, nil 不会 fallback 到默认换 Token 逻辑，而是直接报错。
func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
    switch req.Type {
    case credential.TokenTypeTAT:
        // bot 身份：用 AppID + AppSecret 向飞书换取 tenant_access_token
        token, err := exchangeTenantAccessToken(appID, appSecret)
        if err != nil {
            return nil, err
        }
        return &credential.Token{Value: token, Source: "mycred:tat"}, nil

    case credential.TokenTypeUAT:
        // user 身份：从任意来源获取 user_access_token
        // 例如：从数据库查询、从 Redis 缓存读取、从 OAuth 回调获取、
        // 从配置文件读取，甚至直接写死一个用于测试...
        uat := getUserAccessToken()
        return &credential.Token{Value: uat, Source: "mycred:uat"}, nil
    }
    return nil, nil
}

// getUserAccessToken 从你的来源获取 UAT
// 这里你可以对接任何存储：数据库、Redis、文件、环境变量等
func getUserAccessToken() string {
    // 示例：从环境变量获取
    // return os.Getenv("MY_USER_ACCESS_TOKEN")

    // 示例：从数据库查询
    // return db.Query("SELECT uat FROM tokens WHERE user_id = ?", userID)

    // 示例：写死一个用于本地测试（从 lark-cli auth login 后获取）
    return "u-XXXXXXXX"
}

// exchangeTenantAccessToken 用 AppID + AppSecret 向飞书换取 tenant_access_token
func exchangeTenantAccessToken(appID, appSecret string) (string, error) {
    body, _ := json.Marshal(map[string]string{
        "app_id":     appID,
        "app_secret": appSecret,
    })
    resp, err := http.Post(
        //此处以feishu Brand作为演示，如果为LARK品牌host切换为：open.larksuite.com
        "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
        "application/json", bytes.NewReader(body),
    )
    if err != nil {
        return "", fmt.Errorf("exchange tat: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        Code              int    `json:"code"`
        TenantAccessToken string `json:"tenant_access_token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    if result.Code != 0 {
        return "", fmt.Errorf("exchange tat failed, code=%d", result.Code)
    }
    return result.TenantAccessToken, nil
}

func init() {
    credential.Register(&Provider{})
}