// Public surface of the shared, generated API client. Consumed by apps/web
// today and apps/mobile later — one contract, one typed client. The ./gen tree
// is produced by `moon run proto:generate` (buf) and is git-ignored; never edit
// it by hand. Apps create their own transport (baseUrl differs per platform)
// and call these method descriptors through @connectrpc/connect-query hooks.
export * from './gen/audit/v1/audit_pb'
export * from './gen/audit/v1/audit-AuditService_connectquery'
export * from './gen/auth/v1/auth_pb'
export * from './gen/auth/v1/auth-AuthService_connectquery'
export * from './gen/auth/v1/permission_pb'
// buf.validate.field extension, re-exported (aliased off its generic name) so a
// client can read wire constraints from the contract — e.g. a drift-gate proving
// the hand-written payment-method catalog still equals the proto `in:` list.
export { field as fieldRulesExtension } from './gen/buf/validate/validate_pb'
export * from './gen/notification/v1/notification_pb'
export * from './gen/notification/v1/notification-NotificationService_connectquery'
export * from './gen/payment/v1/payment_pb'
export * from './gen/payment/v1/payment-PaymentService_connectquery'
export * from './gen/report/v1/report_pb'
export * from './gen/report/v1/report-ReportService_connectquery'
export * from './gen/role/v1/role_pb'
export * from './gen/role/v1/role-RoleService_connectquery'
export * from './gen/settings/v1/settings_pb'
export * from './gen/settings/v1/settings-SettingsService_connectquery'
export * from './gen/storage/v1/storage_pb'
export * from './gen/storage/v1/storage-StorageService_connectquery'
export * from './gen/system/v1/system_pb'
export * from './gen/system/v1/system-SystemService_connectquery'
export * from './gen/user/v1/user_pb'
export * from './gen/user/v1/user-UserService_connectquery'
export * from './gen/workflow/v1/workflow_pb'
export * from './gen/workflow/v1/workflow-WorkflowService_connectquery'
