// Shapeable build stub for the vendor telemetry SDKs.
//
// The pinned source imports `@sentry/node` (API) in error paths, cron
// wrappers and integration clients, `@sentry/profiling-node` in its API
// instrument module, and `@sentry/react` in its web instrument and error
// boundary. The shipped board bundles must contain no vendor telemetry code
// and open no vendor endpoint, so the install aliases all three packages to
// this module: every call becomes a no-op that only touches the owner's own
// stderr.
//
// Nothing in the wrapper ever sets SENTRY_DSN, SENTRY_AUTH_TOKEN,
// SENTRY_ORG, SENTRY_PROJECT, VITE_SENTRY_DSN or any sibling variable.
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

export function captureReactException(..._args: unknown[]): string {
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

export function browserTracingIntegration(
  ..._args: unknown[]
): Record<string, never> {
  return {};
}

export function replayIntegration(
  ..._args: unknown[]
): Record<string, never> {
  return {};
}

export function withIsolationScope<T>(fn: () => T): T {
  return fn();
}
