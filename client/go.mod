module jarvis/client

go 1.27.2

require (
	github.com/coder/websocket v1.8.15
	github.com/gen2brain/malgo v0.11.26
	go.yaml.in/yaml/v3 v3.0.4
	jarvis/actions v0.0.0
)

replace jarvis/actions => ../shared/actions
