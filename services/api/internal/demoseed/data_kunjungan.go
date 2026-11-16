package demoseed

import (
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

// Kos Kunjungan Asri is the field organization, for the karyawan's visit list ("Kunjungan hari
// ini"): one kos of 12 rooms, three karyawan who collect from four tenants each, and nobody who
// pays, so every tenant owes about two overdue invoices and opens an H+7 visit task once the daily
// jobs run (admin run-day). One tenant of each karyawan also has two broken promises, which
// GenerateInvoices records (see orgSpec.BrokenPromises), so their visit list shows the
// broken_promises reason next to the overdue one. It belongs to the web's Playwright spec of the
// visit list; change it only together with that spec.

const (
	phoneKunjunganSupervisor = "0812-0010-0001"
	phoneBambang             = "0812-0010-0002"

	propKunjungan = "Kos Asri Kaliurang"
)

func kunjunganOrg() orgSpec {
	// The three karyawan, each collecting from the four tenants listed under their name below.
	const (
		arif  = "0812-0010-0011"
		sri   = "0812-0010-0012"
		teguh = "0812-0010-0013"
	)
	karyawan := func(phone, name string) staffSpec {
		return staffSpec{Phone: phone, Name: name, Grants: orgWide(authz.RoleKaryawan)}
	}
	// Each room is let by the month at the same price, which the lease helper records.
	unitTerms := map[string][]termSpec{}
	room := func(name, phone, unit string, start time.Time, collector string) tenantSpec {
		unitTerms[unit] = monthly(800_000)
		return tenantSpec{Name: name, Phone: phone, Leases: []leaseSpec{
			{Property: propKunjungan, Unit: unit, Start: start, Collector: collector},
		}}
	}

	return orgSpec{
		Name:  OrgKunjungan,
		Actor: phoneBambang,
		Founders: []staffSpec{
			{Phone: phoneKunjunganSupervisor, Name: "Bu Ratna Wulandari", Grants: orgWide(authz.RoleSupervisor)},
			{Phone: phoneBambang, Name: "Pak Bambang Sutrisno", Grants: orgWide(authz.RoleAdmin)},
		},
		Invited: []staffSpec{
			karyawan(arif, "Mas Arif Setiadi"),
			karyawan(sri, "Mbak Sri Lestari"),
			karyawan(teguh, "Mas Teguh Prasetyo"),
		},
		Properties: []propertySpec{{
			Name: propKunjungan, Kind: property.KindKos, Address: "Jl. Kaliurang Km 7 No. 21, Sleman",
			Units: units("Kamar", 1, 12), Terms: monthly(800_000), UnitTerms: unitTerms,
		}},
		Tenants: []tenantSpec{
			// Mas Arif.
			room("Ahmad Fauzi", "0813-0010-0001", "Kamar 1", day(2026, 1, 5), arif),
			room("Bayu Saputra", "0813-0010-0002", "Kamar 2", day(2026, 1, 12), arif),
			room("Citra Ayu Lestari", "0813-0010-0003", "Kamar 3", day(2026, 2, 3), arif),
			room("Danu Prakoso", "0813-0010-0004", "Kamar 4", day(2026, 2, 17), arif),
			// Mbak Sri.
			room("Eko Nugroho", "0813-0010-0005", "Kamar 5", day(2026, 3, 1), sri),
			room("Fajar Ramadhan", "0813-0010-0006", "Kamar 6", day(2026, 3, 14), sri),
			room("Gilang Permana", "0813-0010-0007", "Kamar 7", day(2026, 4, 6), sri),
			room("Hesti Wulansari", "0813-0010-0008", "Kamar 8", day(2026, 4, 20), sri),
			// Mas Teguh.
			room("Imam Santoso", "0813-0010-0009", "Kamar 9", day(2026, 5, 2), teguh),
			room("Joko Susilo", "0813-0010-0010", "Kamar 10", day(2026, 5, 16), teguh),
			room("Kiki Amelia", "0813-0010-0011", "Kamar 11", day(2026, 6, 4), teguh),
			room("Lutfi Hakim", "0813-0010-0012", "Kamar 12", day(2026, 6, 18), teguh),
		},
		// Nobody pays: every tenant stays overdue.
		BrokenPromises: []string{"Citra Ayu Lestari", "Gilang Permana", "Kiki Amelia"},
	}
}
