package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title: "课堂叮咚大屏端", Width: 1180, Height: 760, MinWidth: 420, MinHeight: 600,
		StartHidden:      app.hidden,
		Windows:          &windows.Options{WindowClassName: "ClassDingMainWindow"},
		BackgroundColour: options.NewRGB(247, 249, 244),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "classding-screen-019a124b-8bb6-4f01-89b5-08e59bf482a4",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				if !hasSilentArg(data.Args) {
					go func() { <-app.initDone; app.showWindow() }()
				}
			},
		},
	})
	if err != nil {
		app.shutdown(context.Background())
		log.Fatal(err)
	}
}
