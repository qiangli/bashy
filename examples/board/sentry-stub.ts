// Shapeable build stub for the vendor telemetry SDKs.
//
// The pinned source imports `@sentry/node` (API) in error paths, cron
// wrappers and integration clients, and `@sentry/profiling-node` in its
// instrument module. The shipped board bundle must contain no vendor
// telemetry code and open no vendor endpoint, so the install bundles the
// API with esbuild aliasing both packages to this module: every call
// becomes a no-op that only touches the owner's own stderr.
//
// Nothing in the wrapper ever sets SENTRY_DSN, SENTRY_AUTH_TOKEN,
// SENTRY_ORG, SENTRY_PROJECT, VITE_Sentry_DSN or any sibling variable, so
// even the un-aliased web bundle (which keeps its SDK import) can never
// initialize: both instrument modules skip `init` unless a DSN is set.
// The no-phone-home gate output and the runtime capture in the board
// README are the evidence.

// eslint-disable-next-line @typescript-eslint/no-unused-vars
function ignore(..._args: unknown[]): void {}

export function init(..._args: unknown[]): void {
  ignore(_args);
}

export function captureException(..._args: unknown[]): string {
  return "";
}

export function captureCheckIn(..._args: unknown[]): string {
  return "";
}

export function setUser(..._args: unknown[]): void {
  ignore(_args);
}

export function addBreadcrumb(..._args: unknown[]): void {
  ignore(_args);
}

export function httpIntegration(..._args: unknown[]): Record<string, never> {
  return {};
}

export function nativeNodeFetchIntegration(
  ..._args: unknown[]
): Record<string, never> {
  return {};
}

export function nodeProfilingIntegration(
  ..._args: unknown[]
): Record<string, never> {
  return {};
}

export function withIsolationScope<T>(fn: () => T): T {
  return fn();
}
