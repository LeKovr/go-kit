package main

import "context"

type service struct{}

func (*service) Health(context.Context) (string, error) { return "ok", nil }
