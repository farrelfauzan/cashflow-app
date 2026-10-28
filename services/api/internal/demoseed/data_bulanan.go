package demoseed

import (
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

// Kos Bulanan Sejahtera is the volume organization: 27 members, 25 properties and 45 tenants, so
// the members, properties and tenant lists span several pages of 20. Its three main properties
// hold the tenants; the others are small, with varied names for searching.

const (
	// Its own admins, members of this organization only, so the web's specs for the other two
	// organizations keep their organization pickers as they are.
	phoneDimas             = "0812-0009-0027"
	phoneLestari           = "0812-0009-0028"
	phoneBulananSupervisor = "0812-0009-0002"

	propBulananKos     = "Kos Sejahtera Utama"
	propBulananKontrak = "Kontrakan Seturan"
	propBulananRuko    = "Ruko Condongcatur"
)

// The booking dates are fixed and far ahead, so a booked lease stays booked (it starts after
// today) for a long time. They pass in June 2027; change them together with the web specs.
var (
	bookedJune2027 = day(2027, 6, 1)
	bookedJuly2027 = day(2027, 7, 1)
)

func bulananOrg() orgSpec {
	// Active karyawan who collect rent, and two disabled ones who still collect on running
	// leases (the "belum punya penagih" flag).
	const (
		anton   = "0812-0009-0006"
		dewi    = "0812-0009-0007"
		fitri   = "0812-0009-0009"
		galih   = "0812-0009-0010"
		heni    = "0812-0009-0011"
		juwita  = "0812-0009-0013"
		kurnia  = "0812-0009-0014"
		lina    = "0812-0009-0015"
		disEko  = "0812-0009-0008"
		disImam = "0812-0009-0012"
	)
	karyawan := func(phone, name string, disabled bool) staffSpec {
		return staffSpec{Phone: phone, Name: name, Grants: orgWide(authz.RoleKaryawan), Disabled: disabled}
	}
	kos := func(unit string, start time.Time, rent int64, collector string) leaseSpec {
		return leaseSpec{Property: propBulananKos, Unit: unit, Start: start, Rent: rent, Collector: collector}
	}
	kontrakan := func(unit string, start time.Time, rent int64, collector string) leaseSpec {
		return leaseSpec{Property: propBulananKontrak, Unit: unit, Start: start, Rent: rent, Collector: collector}
	}
	ruko := func(unit string, start time.Time, rent int64, collector string) leaseSpec {
		return leaseSpec{Property: propBulananRuko, Unit: unit, Start: start, Rent: rent, Collector: collector}
	}
	ended := func(l leaseSpec, lastDay time.Time) leaseSpec {
		l.MovedOut = lastDay
		return l
	}
	solo := func(name, phone string, l leaseSpec) tenantSpec {
		return tenantSpec{Name: name, Phone: phone, Leases: []leaseSpec{l}}
	}

	return orgSpec{
		Name:  OrgBulanan,
		Actor: phoneDimas,
		Founders: []staffSpec{
			{Phone: phoneBulananSupervisor, Name: "Bu Maya Kusumawati", Grants: orgWide(authz.RoleSupervisor)},
			{Phone: phoneDimas, Name: "Pak Dimas Prakoso", Grants: orgWide(authz.RoleAdmin)},
		},
		Invited: []staffSpec{
			{Phone: phoneLestari, Name: "Mbak Lestari Anjani", Grants: []grantSpec{
				{Role: authz.RoleAdmin, Property: propBulananKontrak},
			}},
			{Phone: "0812-0009-0003", Name: "Pak Suryanto Wibisono", Grants: orgWide(authz.RoleAdmin)},
			{Phone: "0812-0009-0004", Name: "Bu Retno Handayani", Grants: []grantSpec{
				{Role: authz.RoleAdmin, Property: propBulananKos},
			}},
			// An admin who left, limited to one property, with no leases.
			{
				Phone: "0812-0009-0005", Name: "Mas Bayu Firmansyah", Disabled: true,
				Grants: []grantSpec{{Role: authz.RoleAdmin, Property: propBulananRuko}},
			},
			karyawan(anton, "Mas Anton Prasetyo", false),
			karyawan(dewi, "Mbak Dewi Puspita", false),
			karyawan(disEko, "Mas Eko Susilo", true),
			karyawan(fitri, "Mbak Fitri Handayani", false),
			karyawan(galih, "Mas Galih Pradana", false),
			karyawan(heni, "Mbak Heni Marlina", false),
			karyawan(disImam, "Mas Imam Santoso", true),
			karyawan(juwita, "Mbak Juwita Sari", false),
			karyawan(kurnia, "Mas Kurnia Aji", false),
			karyawan(lina, "Mbak Lina Rosita", false),
			karyawan("0812-0009-0016", "Mas Mahendra Putra", false),
			karyawan("0812-0009-0017", "Mbak Nia Ramadhani", true), // left, no leases
			karyawan("0812-0009-0018", "Mas Panji Laksana", false),
			karyawan("0812-0009-0019", "Mbak Rani Puspasari", false),
			karyawan("0812-0009-0020", "Mas Sigit Purnomo", false),
			karyawan("0812-0009-0021", "Mbak Tika Amelia", false),
			karyawan("0812-0009-0022", "Mas Udin Saefudin", false),
			karyawan("0812-0009-0023", "Mbak Vera Yuliana", false),
			karyawan("0812-0009-0024", "Mas Wawan Setiyawan", false),
			karyawan("0812-0009-0025", "Mbak Yanti Susilowati", false),
			karyawan("0812-0009-0026", "Mas Zaenal Arifin", false),
		},
		Properties: append([]propertySpec{
			{
				Name: propBulananKos, Kind: property.KindKos, Address: "Jl. Kaliurang Km 8,5 No. 30, Sleman, Yogyakarta",
				Units: units("Kamar", 1, 30), Rent: 900_000,
			},
			{
				Name: propBulananKontrak, Kind: property.KindKontrakan, Address: "Jl. Seturan Raya No. 14, Sleman, Yogyakarta",
				Units: units("Rumah", 1, 12), Rent: 1_750_000,
			},
			{
				Name: propBulananRuko, Kind: property.KindRuko, Address: "Jl. Raya Condongcatur No. 52, Sleman, Yogyakarta",
				Units: units("Ruko", 1, 6), Rent: 5_000_000,
			},
		}, bulananSmallProperties()...),
		Tenants: []tenantSpec{
			// Kos Sejahtera Utama: 22 running leases (Kamar 7 and 14 are collected by Eko, who has left).
			solo("Andi Pratama", "0813-0009-0001", kos("Kamar 1", day(2025, 11, 1), 850_000, anton)),
			solo("Bagas Aditya", "0813-0009-0002", kos("Kamar 2", day(2025, 12, 1), 850_000, anton)),
			solo("Cahyo Wibowo", "0813-0009-0003", kos("Kamar 3", day(2026, 1, 10), 900_000, dewi)),
			solo("Dian Permata Sari", "0813-0009-0004", kos("Kamar 4", day(2026, 1, 15), 900_000, dewi)),
			solo("Eka Putri Ramadhani", "0813-0009-0005", kos("Kamar 5", day(2026, 2, 1), 900_000, "")),
			solo("Farhan Maulana", "0813-0009-0006", kos("Kamar 6", day(2026, 2, 20), 950_000, fitri)),
			solo("Gita Savitri", "0813-0009-0007", kos("Kamar 7", day(2026, 3, 1), 900_000, disEko)),
			solo("Hadi Susanto", "0813-0009-0008", kos("Kamar 8", day(2026, 3, 5), 900_000, fitri)),
			solo("Indah Kusuma Wardani", "0813-0009-0009", kos("Kamar 9", day(2026, 3, 25), 1_000_000, galih)),
			solo("Joni Hermawan", "0813-0009-0010", kos("Kamar 10", day(2026, 4, 1), 900_000, galih)),
			solo("Kartika Dewi", "0813-0009-0011", kos("Kamar 11", day(2026, 4, 12), 900_000, heni)),
			solo("Lukman Hakim", "0813-0009-0012", kos("Kamar 12", day(2026, 5, 1), 850_000, "")),
			solo("Maya Anggraeni", "0813-0009-0013", kos("Kamar 13", day(2026, 5, 17), 1_000_000, heni)),
			solo("Nanda Ardiansyah", "0813-0009-0014", kos("Kamar 14", day(2026, 6, 1), 900_000, disEko)),
			solo("Oky Firmansyah", "0813-0009-0015", kos("Kamar 15", day(2026, 6, 8), 900_000, juwita)),
			solo("Putri Rahayu", "0813-0009-0016", kos("Kamar 16", day(2026, 7, 1), 950_000, juwita)),
			solo("Qori Amalia", "0813-0009-0017", kos("Kamar 17", day(2026, 7, 14), 900_000, kurnia)),
			solo("Rizki Ananda", "0813-0009-0018", kos("Kamar 18", day(2026, 8, 1), 850_000, kurnia)),
			solo("Sinta Maharani", "0813-0009-0019", kos("Kamar 19", day(2026, 8, 20), 900_000, lina)),
			solo("Tegar Wijaya", "0813-0009-0020", kos("Kamar 20", day(2026, 9, 1), 900_000, lina)),
			solo("Umi Kulsum", "0813-0009-0021", kos("Kamar 21", day(2026, 9, 15), 1_100_000, anton)),
			solo("Vina Oktaviani", "0813-0009-0022", kos("Kamar 22", day(2026, 10, 1), 900_000, "")),
			// Three bookings, far enough ahead to stay in the future.
			solo("Wahyu Hidayat", "0813-0009-0023", kos("Kamar 23", bookedJune2027, 900_000, "")),
			solo("Yoga Permana", "0813-0009-0024", kos("Kamar 24", bookedJune2027, 900_000, dewi)),
			solo("Zahra Nabila", "0813-0009-0025", kos("Kamar 25", bookedJuly2027, 950_000, "")),
			// Two tenants who moved out.
			solo("Agung Laksono", "0813-0009-0026", ended(kos("Kamar 26", day(2026, 2, 1), 900_000, ""), day(2026, 8, 31))),
			solo("Bella Safitri", "0813-0009-0027", ended(kos("Kamar 27", day(2025, 10, 1), 850_000, ""), day(2026, 9, 30))),
			// Two with no lease yet.
			{Name: "Dhea Ananda Putri", Phone: "0813-0009-0028"},
			{Name: "Eri Kurniawan", Phone: "0813-0009-0029"},

			// Kontrakan Seturan, the property Mbak Lestari is limited to: nine tenants, Heri's
			// collector is Imam, who has left.
			solo("Fitriani Nur Hasanah", "0813-0009-0030", kontrakan("Rumah 1", day(2026, 1, 1), 1_750_000, anton)),
			solo("Gunawan Santoso", "0813-0009-0031", kontrakan("Rumah 2", day(2026, 2, 1), 1_750_000, dewi)),
			solo("Heri Setiawan", "0813-0009-0032", kontrakan("Rumah 3", day(2026, 3, 15), 1_800_000, disImam)),
			solo("Irfan Hakim", "0813-0009-0033", kontrakan("Rumah 4", day(2026, 5, 1), 1_750_000, fitri)),
			solo("Jumiati", "0813-0009-0034", kontrakan("Rumah 5", day(2026, 6, 10), 1_500_000, "")),
			solo("Kurniawan Adi", "0813-0009-0035", kontrakan("Rumah 6", day(2026, 8, 1), 1_750_000, galih)),
			solo("Laila Fitria", "0813-0009-0036", kontrakan("Rumah 7", day(2026, 9, 1), 2_000_000, heni)),
			solo("Mulyadi", "0813-0009-0037", kontrakan("Rumah 8", bookedJune2027, 1_750_000, "")),
			solo("Novita Rahmawati", "0813-0009-0038", ended(kontrakan("Rumah 9", day(2026, 1, 1), 1_750_000, ""), day(2026, 9, 30))),

			// Ruko Condongcatur.
			solo("Toko Berkah Jaya (Koh Aming)", "0813-0009-0039", ruko("Ruko 1", day(2025, 10, 1), 5_000_000, kurnia)),
			solo("Warung Makan Bu Tini", "0813-0009-0040", ruko("Ruko 2", day(2026, 3, 1), 3_750_000, lina)),
			solo("Klinik Sehat Sentosa", "0813-0009-0041", ruko("Ruko 3", day(2026, 4, 15), 6_500_000, "")),
			solo("Laundry Kilat (Pak Darmawan)", "0813-0009-0042", ruko("Ruko 4", day(2026, 7, 1), 4_500_000, juwita)),
			solo("Barbershop Gaya Muda", "0813-0009-0043", ruko("Ruko 5", bookedJuly2027, 5_000_000, "")),
			solo("Fotokopi Maju Terus", "0813-0009-0044", ended(ruko("Ruko 6", day(2025, 11, 1), 4_000_000, ""), day(2026, 9, 30))),
			{Name: "Apotek Waras Medika", Phone: "0813-0009-0045"},
		},
	}
}

// bulananSmallProperties are the 22 small properties with one to three units each. Names repeat
// words on purpose ("Melati", "Cempaka"), so a name search finds several.
func bulananSmallProperties() []propertySpec {
	kos := func(name, address string, rooms int, rent int64) propertySpec {
		return propertySpec{Name: name, Kind: property.KindKos, Address: address, Units: units("Kamar", 1, rooms), Rent: rent}
	}
	kontrakan := func(name, address string, houses int, rent int64) propertySpec {
		return propertySpec{Name: name, Kind: property.KindKontrakan, Address: address, Units: units("Rumah", 1, houses), Rent: rent}
	}
	ruko := func(name, address string, shops int, rent int64) propertySpec {
		return propertySpec{Name: name, Kind: property.KindRuko, Address: address, Units: units("Ruko", 1, shops), Rent: rent}
	}
	return []propertySpec{
		kos("Kos Melati 2", "Jl. Kaliurang Km 6 No. 3, Sleman", 3, 800_000),
		kos("Kos Melati 3", "Jl. Kaliurang Km 6 No. 9, Sleman", 2, 800_000),
		kos("Kos Mawar Putih", "Jl. Pogung Baru No. 18, Sleman", 3, 750_000),
		kos("Kos Dahlia", "Jl. Gejayan Gg. Dahlia No. 4, Sleman", 2, 850_000),
		kos("Kos Kenanga", "Jl. Monjali No. 27, Sleman", 3, 700_000),
		kos("Kos Flamboyan", "Jl. Colombo No. 61, Sleman", 2, 1_000_000),
		kos("Kos Teratai", "Jl. Affandi No. 15, Sleman", 3, 950_000),
		kos("Kos Putra Sagan", "Jl. Sagan No. 8, Yogyakarta", 3, 650_000),
		kos("Kos Eksklusif Babarsari", "Jl. Babarsari No. 102, Sleman", 2, 1_800_000),
		kos("Kos Cempaka Putih", "Jl. Gamping Lor No. 5, Sleman", 1, 600_000),
		kontrakan("Kontrakan Cempaka", "Jl. Wates Km 4 No. 22, Yogyakarta", 2, 1_400_000),
		kontrakan("Kontrakan Nusa Indah", "Jl. Nusa Indah No. 11, Bantul", 2, 1_200_000),
		kontrakan("Kontrakan Pogung", "Jl. Pogung Lor No. 30, Sleman", 1, 1_600_000),
		kontrakan("Kontrakan Tegalrejo", "Jl. Magelang Km 3 No. 40, Yogyakarta", 2, 1_300_000),
		kontrakan("Kontrakan Jetis", "Jl. Jetis No. 9, Yogyakarta", 3, 1_100_000),
		kontrakan("Kontrakan Mlati Asri", "Jl. Magelang Km 8 No. 6, Sleman", 2, 1_500_000),
		ruko("Ruko Cempaka", "Jl. Gejayan No. 77, Sleman", 2, 4_000_000),
		ruko("Ruko Kaliurang", "Jl. Kaliurang Km 9 No. 2, Sleman", 3, 4_500_000),
		ruko("Ruko Gejayan", "Jl. Gejayan No. 120, Sleman", 2, 5_500_000),
		ruko("Ruko Maguwo", "Jl. Raya Solo Km 9 No. 14, Sleman", 1, 3_500_000),
		ruko("Ruko Jakal Km 7", "Jl. Kaliurang Km 7 No. 50, Sleman", 2, 6_000_000),
		{
			Name: "Paviliun Sinduadi", Kind: property.KindOther, Address: "Jl. Sinduadi No. 3, Sleman",
			Units: []string{"Paviliun 1"}, Rent: 2_000_000,
		},
	}
}
