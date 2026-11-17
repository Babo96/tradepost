package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFiles embed.FS

func (s *Server) webHandler() http.Handler {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
