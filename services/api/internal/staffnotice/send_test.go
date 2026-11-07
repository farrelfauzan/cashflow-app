package staffnotice

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

func TestUnits(t *testing.T) {
	t.Parallel()
	lease := func(status, unit, property string) dbgen.ListNoticeLeasesRow {
		return dbgen.ListNoticeLeasesRow{ID: uuid.New(), Status: status, UnitLabel: unit, PropertyName: property}
	}
	tests := []struct {
		name   string
		leases []dbgen.ListNoticeLeasesRow
		want   string
	}{
		{name: "one active lease", leases: []dbgen.ListNoticeLeasesRow{lease("active", "Kamar 3", "Kos Melati")}, want: "Kamar 3, Kos Melati"},
		{
			name: "two active leases, the ended one left out",
			leases: []dbgen.ListNoticeLeasesRow{
				lease("active", "Kamar 3", "Kos Melati"), lease("active", "Ruko 2", "Ruko Sentosa"), lease("ended", "Kamar 1", "Kos Melati"),
			},
			want: "Kamar 3, Kos Melati; Ruko 2, Ruko Sentosa",
		},
		{
			name:   "moved out: the leases they had, each unit once",
			leases: []dbgen.ListNoticeLeasesRow{lease("ended", "Kamar 1", "Kos Melati"), lease("ended", "Kamar 1", "Kos Melati")},
			want:   "Kamar 1, Kos Melati",
		},
		{name: "no lease", want: "tanpa unit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, units(tc.leases))
		})
	}
}

func TestBrokenPromisesReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{name: "the threshold", n: 2, want: "janji bayar tidak ditepati 2 kali"},
		{name: "more since the task opened", n: 3, want: "janji bayar tidak ditepati 3 kali"},
		{name: "none counted", n: 0, want: "beberapa janji bayar tidak ditepati"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, brokenPromisesReason(tc.n))
		})
	}
}
