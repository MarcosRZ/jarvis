module jarvis/collector

go 1.27.2

require (
	github.com/coder/websocket v1.8.15
	jarvis/actions v0.0.0
)

replace jarvis/actions => ../shared/actions
