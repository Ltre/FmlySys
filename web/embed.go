package web

import "embed"

// FS contains HTML templates and static assets.
//
//go:embed templates/*.html static/*
var FS embed.FS
