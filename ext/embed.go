// Package ext embeds the pi extension that every driver job loads with `-e`.
// `driver serve` writes DriverTS to ~/.driver/ext/driver.ts on each start so the
// file on disk always matches the running binary.
package ext

import _ "embed"

//go:embed driver.ts
var DriverTS []byte
