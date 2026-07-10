package server

import (
	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/gen/audit/v1/auditv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/notification/v1/notificationv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/payment/v1/paymentv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/report/v1/reportv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/role/v1/rolev1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/settings/v1/settingsv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/system/v1/systemv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/user/v1/userv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/workflow/v1/workflowv1connect"
)

// authzRules is the single access-control decision table: every RPC procedure
// maps to Public, "any authenticated user" (empty Permission), or a required
// permission from the auth.Catalog. authz_test.go asserts this table covers
// every registered service method, so a new RPC without an entry fails CI.
var authzRules = map[string]auth.Rule{
	// auth.v1 — session lifecycle. Login/Register/GetAuthConfig must be
	// reachable logged-out; Logout is public so a stale cookie can always be
	// cleared; profile/password require a session but no permission.
	authv1connect.AuthServiceLoginProcedure:          {Public: true},
	authv1connect.AuthServiceLogoutProcedure:         {Public: true},
	authv1connect.AuthServiceRegisterProcedure:       {Public: true},
	authv1connect.AuthServiceGetAuthConfigProcedure:  {Public: true},
	authv1connect.AuthServiceGetMeProcedure:          {},
	authv1connect.AuthServiceUpdateProfileProcedure:  {},
	authv1connect.AuthServiceChangePasswordProcedure: {},

	// Channel-backed auth flows. Reset/verify/sms-login run logged-out by
	// nature; the send RPCs for the current user require a session. Register
	// codes are public (pre-account) and captcha-gated in the handler.
	authv1connect.AuthServiceSendVerificationEmailProcedure: {},
	authv1connect.AuthServiceVerifyEmailProcedure:           {Public: true},
	authv1connect.AuthServiceRequestPasswordResetProcedure:  {Public: true},
	authv1connect.AuthServiceResetPasswordProcedure:         {Public: true},
	authv1connect.AuthServiceSendPhoneBindCodeProcedure:     {},
	authv1connect.AuthServiceBindPhoneProcedure:             {},
	authv1connect.AuthServiceSendSmsLoginCodeProcedure:      {Public: true},
	authv1connect.AuthServiceLoginWithSmsProcedure:          {Public: true},
	authv1connect.AuthServiceSendPhoneRegisterCodeProcedure: {Public: true},
	authv1connect.AuthServiceSendEmailRegisterCodeProcedure: {Public: true},
	authv1connect.AuthServiceSendEmailBindCodeProcedure:     {},
	authv1connect.AuthServiceBindEmailProcedure:             {},
	authv1connect.AuthServiceUnbindPhoneProcedure:           {},
	authv1connect.AuthServiceUnbindEmailProcedure:           {},
	authv1connect.AuthServiceListMySessionsProcedure:        {},
	authv1connect.AuthServiceRevokeMySessionProcedure:       {},
	authv1connect.AuthServiceCompleteOauthSignupProcedure:   {Public: true},
	authv1connect.AuthServiceListMyIdentitiesProcedure:      {},
	authv1connect.AuthServiceUnbindOauthIdentityProcedure:   {},
	authv1connect.AuthServiceSetupTotpProcedure:             {},
	authv1connect.AuthServiceActivateTotpProcedure:          {},
	authv1connect.AuthServiceDisableTotpProcedure:           {},
	authv1connect.AuthServiceLoginWithTotpProcedure:         {Public: true},

	// notification.v1 — the per-user inbox; every RPC is self-scoped to the
	// caller's session (any authenticated user, no permission gate).
	notificationv1connect.NotificationServiceListNotificationsProcedure:        {},
	notificationv1connect.NotificationServiceGetUnreadCountProcedure:           {},
	notificationv1connect.NotificationServiceMarkNotificationsReadProcedure:    {},
	notificationv1connect.NotificationServiceMarkAllNotificationsReadProcedure: {},
	notificationv1connect.NotificationServiceDeleteNotificationProcedure:       {},

	// report.v1 — read-only dashboard aggregates.
	reportv1connect.ReportServiceGetDashboardReportProcedure: {Permission: "report.read"},

	// user.v1 — admin user management.
	userv1connect.UserServiceListUsersProcedure:         {Permission: "user.read"},
	userv1connect.UserServiceCreateUserProcedure:        {Permission: "user.write"},
	userv1connect.UserServiceUpdateUserProcedure:        {Permission: "user.write"},
	userv1connect.UserServiceDeleteUserProcedure:        {Permission: "user.write"},
	userv1connect.UserServiceResetUserPasswordProcedure: {Permission: "user.write"},

	// role.v1 — RBAC management. ListPermissions is readable by role.read
	// since the role editor needs the catalog to render checkboxes.
	rolev1connect.RoleServiceListRolesProcedure:       {Permission: "role.read"},
	rolev1connect.RoleServiceListPermissionsProcedure: {Permission: "role.read"},
	rolev1connect.RoleServiceCreateRoleProcedure:      {Permission: "role.write"},
	rolev1connect.RoleServiceUpdateRoleProcedure:      {Permission: "role.write"},
	rolev1connect.RoleServiceDeleteRoleProcedure:      {Permission: "role.write"},

	// settings.v1 — business settings. GetSiteInfo is public: the login page
	// and document head render the site identity before any session exists.
	settingsv1connect.SettingsServiceGetSettingsProcedure:    {Permission: "settings.read"},
	settingsv1connect.SettingsServiceUpdateSettingsProcedure: {Permission: "settings.write"},
	settingsv1connect.SettingsServiceGetSiteInfoProcedure:    {Public: true},

	// system.v1 — infrastructure integrations (secrets); separate persona from
	// business settings, hence separate permissions.
	systemv1connect.SystemServiceGetSystemSettingsProcedure:        {Permission: "system.read"},
	systemv1connect.SystemServiceDescribeStorageProvidersProcedure: {Permission: "system.read"},
	systemv1connect.SystemServiceCreateStorageProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateStorageProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteStorageProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceBindStoragePurposeProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceDescribeCaptchaProvidersProcedure: {Permission: "system.read"},
	systemv1connect.SystemServiceCreateCaptchaProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateCaptchaProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteCaptchaProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceBindCaptchaPurposeProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceDescribeEmailProvidersProcedure:   {Permission: "system.read"},
	systemv1connect.SystemServiceCreateEmailProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateEmailProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteEmailProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceBindEmailPurposeProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceDescribeSmsProvidersProcedure:     {Permission: "system.read"},
	systemv1connect.SystemServiceCreateSmsProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateSmsProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteSmsProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceBindSmsPurposeProcedure:           {Permission: "system.write"},
	systemv1connect.SystemServiceDescribeLlmProvidersProcedure:     {Permission: "system.read"},
	systemv1connect.SystemServiceCreateLlmProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateLlmProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteLlmProfileProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceBindLlmPurposeProcedure:           {Permission: "system.write"},
	systemv1connect.SystemServiceDescribeOauthProvidersProcedure:   {Permission: "system.read"},
	systemv1connect.SystemServiceCreateOauthProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceUpdateOauthProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceDeleteOauthProfileProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceBindOauthPurposeProcedure:         {Permission: "system.write"},
	systemv1connect.SystemServiceDescribePaymentProvidersProcedure: {Permission: "system.read"},
	systemv1connect.SystemServiceCreatePaymentProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceUpdatePaymentProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceDeletePaymentProfileProcedure:     {Permission: "system.write"},
	systemv1connect.SystemServiceBindPaymentPurposeProcedure:       {Permission: "system.write"},
	systemv1connect.SystemServiceTestStorageConnectionProcedure:    {Permission: "system.write"},
	systemv1connect.SystemServiceSendTestEmailProcedure:            {Permission: "system.write"},
	systemv1connect.SystemServiceSendTestSmsProcedure:              {Permission: "system.write"},
	systemv1connect.SystemServiceTestLlmProcedure:                  {Permission: "system.write"},

	// storage.v1 — any authenticated user may upload their own avatar; site
	// branding uploads belong to the business-settings persona.
	storagev1connect.StorageServicePresignAvatarUploadProcedure:    {},
	storagev1connect.StorageServicePresignSiteAssetUploadProcedure: {Permission: "settings.write"},

	// workflow.v1 — durable-run observability and control.
	workflowv1connect.WorkflowServiceListWorkflowRunsProcedure:    {Permission: "workflow.read"},
	workflowv1connect.WorkflowServiceGetWorkflowRunProcedure:      {Permission: "workflow.read"},
	workflowv1connect.WorkflowServiceCancelWorkflowRunProcedure:   {Permission: "workflow.write"},
	workflowv1connect.WorkflowServiceResumeWorkflowRunProcedure:   {Permission: "workflow.write"},
	workflowv1connect.WorkflowServiceTriggerDemoWorkflowProcedure: {Permission: "workflow.write"},

	// audit.v1 — read-only trail of admin actions; writes happen only in the
	// audit interceptor, so one read permission is the whole surface.
	auditv1connect.AuditServiceListAuditLogsProcedure: {Permission: "audit.read"},

	// payment.v1 — signed checkout sessions are public capabilities; management
	// and the demo issuer remain permissioned back-office operations.
	paymentv1connect.PaymentCheckoutServiceGetCheckoutSessionProcedure: {Public: true},
	paymentv1connect.PaymentCheckoutServicePlanCheckoutProcedure:       {Public: true},
	paymentv1connect.PaymentCheckoutServiceConfirmCheckoutProcedure:    {Public: true},
	paymentv1connect.PaymentCheckoutServiceGetCheckoutOrderProcedure:   {Public: true},
	paymentv1connect.PaymentServiceCreateDemoCheckoutProcedure: {
		Permission: "payment.write",
	},
	paymentv1connect.PaymentServiceGetPaymentOrderProcedure: {
		Permission: "payment.read",
	},
	paymentv1connect.PaymentServiceSyncPaymentOrderProcedure: {
		Permission: "payment.write",
	},
	paymentv1connect.PaymentServiceListPaymentOrdersProcedure: {
		Permission: "payment.read",
	},
	paymentv1connect.PaymentServiceRefundPaymentOrderProcedure: {
		Permission: "payment.write",
	},
}
