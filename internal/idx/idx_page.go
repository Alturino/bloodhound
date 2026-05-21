package idx

import (
	"context"
	"log/slog"
)

type Page struct {
	Index         int             `json:"index"`
	Total         int             `json:"total_page"`
	Ctx           context.Context `json:"-"`
	Announcements []Announcement  `json:"-"`
	Params        SearchParams    `json:"search_params"`
}

func (p *Page) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("index", p.Index),
		slog.Int("total_page", p.Total),
		slog.Any("search_params", p.Params),
	)
}
