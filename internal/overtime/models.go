package overtime

import "time"

// Entry is exported because internal/sync reads rows directly (via
// EntryChangesSince) to build the unified /sync/changes feed.
type Entry struct {
	ID                      string     `json:"id"`
	UserID                  string     `json:"-"`
	Fecha                   string     `json:"fecha"`
	SolicitadaPor           string     `json:"solicitada_por"`
	Actividad               string     `json:"actividad"`
	Observaciones           string     `json:"observaciones"`
	HoraInicio              string     `json:"hora_inicio"`
	HoraFinal               string     `json:"hora_final"`
	TotalHoras              float64    `json:"total_horas"`
	ExtrasDiurnas           float64    `json:"extras_diurnas"`
	ExtrasNocturnas         float64    `json:"extras_nocturnas"`
	ExtrasDiurnasFestivas   float64    `json:"extras_diurnas_festivas"`
	ExtrasNocturnasFestivas float64    `json:"extras_nocturnas_festivas"`
	Seq                     int64      `json:"seq"`
	UpdatedAt               time.Time  `json:"updated_at"`
	DeletedAt               *time.Time `json:"deleted_at,omitempty"`
}

// MonthMeta is keyed by (user_id, year_month) — no client-generated
// id, unlike Entry. YearMonth acts as the synthetic id in REST URLs
// and sync changes.
type MonthMeta struct {
	UserID      string     `json:"-"`
	YearMonth   string     `json:"year_month"`
	Colaborador string     `json:"colaborador"`
	Cedula      string     `json:"cedula"`
	Seq         int64      `json:"seq"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}
