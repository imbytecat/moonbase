//go:build !embed

package web

import "net/http"

const Enabled = false

func Handler() http.Handler { return nil }
