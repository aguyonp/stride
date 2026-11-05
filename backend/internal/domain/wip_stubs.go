package domain

import "context"

func (s *Services) runSync(ctx context.Context, uid int64, morning bool) error { return nil }

func (s *Services) ConnectGarmin(ctx context.Context, uid int64, key string) (any, error) { return nil, nil }

func errText(err error) string { return err.Error() }

func (s *Services) dropClient(uid int64) {}
