package mocks

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

type DHPSAImportClientMock struct {
	PSAImportFn func(context.Context, []dh.PSAImportItem) (*dh.PSAImportResponse, error)
	Calls       int
}

func (m *DHPSAImportClientMock) PSAImport(c context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
	m.Calls++
	if m.PSAImportFn != nil {
		return m.PSAImportFn(c, items)
	}
	return &dh.PSAImportResponse{}, nil
}
