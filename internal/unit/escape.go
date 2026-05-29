// Package unit generates systemd unit file content from schedule configs.
package unit

import "strings"

// EscapeSpecifiers replaces % with %% to prevent systemd specifier expansion.
// Used for Description, WorkingDirectory, EnvironmentFile, X-WakeConfig.
func EscapeSpecifiers(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// EscapeExecDirect replaces % with %% and $ with $$ for ExecStart directives
// that run a binary directly (no shell wrapper).
func EscapeExecDirect(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "$", "$$")
	return s
}

// EscapeExecShell replaces % with %% for ExecStart directives that use
// /bin/sh -c. Leaves $ alone because the shell needs it. Does NOT escape "
// because resolveExec already handles quoting for systemd's ExecStart parsing.
func EscapeExecShell(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// QuoteEnvAssignment formats a key=value pair as a double-quoted Environment=
// directive value. Both key and value have specifiers escaped.
func QuoteEnvAssignment(key, value string) string {
	return `"` + EscapeSpecifiers(key) + "=" + EscapeSpecifiers(value) + `"`
}
