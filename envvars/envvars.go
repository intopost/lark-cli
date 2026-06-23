// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

const (
	CliAppID             = "LARKSUITE_CLI_APP_ID"
	CliAppSecret         = "LARKSUITE_CLI_APP_SECRET"
	CliBrand             = "LARKSUITE_CLI_BRAND"
	CliUserAccessToken   = "LARKSUITE_CLI_USER_ACCESS_TOKEN"
	CliTenantAccessToken = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"
	CliDefaultAs         = "LARKSUITE_CLI_DEFAULT_AS"
	CliStrictMode        = "LARKSUITE_CLI_STRICT_MODE"

	// Sidecar proxy (auth proxy mode)
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY"
	CliProxyKey  = "LARKSUITE_CLI_PROXY_KEY"

	// Content safety scanning mode
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"

	CliProxyEnable  = "LARKSUITE_CLI_PROXY_ENABLE"
	CliProxyAddress = "LARKSUITE_CLI_PROXY_ADDRESS"
	CliCAPath       = "LARKSUITE_CLI_CA_PATH"

	AIPowerBaseURL = "AIPOWER_POWER_URL"
	AIPowerAPIToken = "AIPOWER_API_TOKEN"

	IPassSessionID = "IPASS_SESSION_ID"
	IPassRunID     = "IPASS_RUN_ID"
	IPassTeamUUID  = "IPASS_TEAM_UUID"

	LarkCLIOCAdapterURL = "LARK_CLI_OC_ADAPTER_URL"
)
