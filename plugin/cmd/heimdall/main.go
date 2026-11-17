// Command heimdall is the shared-object entrypoint Bifrost loads:
//
//	go build -buildmode=plugin -o heimdall.so ./cmd/heimdall
//
// It forwards Bifrost's exported hook symbols to the heimdall package.
package main

import (
	"errors"

	heimdall "github.com/Chinteyley/heimdall/plugin"
	"github.com/maximhq/bifrost/core/schemas"
)

var p *heimdall.Plugin

func Init(config any) error {
	var err error
	p, err = heimdall.New(config)
	return err
}

func GetName() string { return "heimdall" }

func Cleanup() error { return nil }

var errNotInit = errors.New("heimdall: Init was not called")

func HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	if p == nil {
		return nil, errNotInit
	}
	return p.HTTPTransportPreHook(ctx, req)
}

func HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	if p == nil {
		return errNotInit
	}
	return p.HTTPTransportPostHook(ctx, req, resp)
}

func main() {}
