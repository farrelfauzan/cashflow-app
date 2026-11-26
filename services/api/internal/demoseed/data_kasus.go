package demoseed

import (
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Kos Aneka Kasus is the organization of every field and payment case (T-109): one tenant per
// case, so the demo walks each screen's states without preparing data by hand. Unlike the other
// organizations its dates are relative to the day it is seeded (a case such as "due in two days"
// cannot be a fixed date), so its cases are as described only on that day; reseed after
// dev-reset for a fresh demo. seed-demo creates its people, units and leases; SeedCases, run by
// generate-invoices, bills them and records each case through the app's own services. No web
// spec depends on it.

// OrgKasus is the case organization's name.
const OrgKasus = "Kos Aneka Kasus"

// Its staff. Yoga and Dina are the two karyawan: Yoga collects from almost every tenant, Dina from
// two, so Yoga's lists show what another karyawan's tenants look like (absent, and 403).
const (
	phoneKasusSupervisor = "0812-0011-0001"
	phoneKasusAdmin      = "0812-0011-0002"
	phoneYoga            = "0812-0011-0011"
	phoneDina            = "0812-0011-0012"

	propKasusKos       = "Kos Aneka Jakal"
	propKasusKontrakan = "Kontrakan Aneka Pogung"

	kasusRoomRent  = 900_000
	kasusHouseRent = 1_500_000
)

// The tenants, one per case (the case numbers are T-109's). Every one is collected by Yoga unless
// it says Dina.
const (
	KasusDueSoon        = "Rizky Ananda"      // 1: due in 2 days, earlier invoices paid
	KasusNotDue         = "Salsabila Putri"   // 2: next invoice due in 5 days, nothing overdue
	KasusQRISFull       = "Taufik Hidayat"    // 3: paid everything by QRIS
	KasusQRISPart       = "Umar Said"         // 4: paid half by QRIS
	KasusCredit         = "Vina Melati"       // 5: overpaid 10 days ago; the credit paid part of the next invoice
	KasusCashToday      = "Wahyu Kurniawan"   // 6: paid Yoga everything in cash today (needs storage)
	KasusCashOther      = "Yusuf Maulana"     // 7: Dina's; paid Dina in cash today (needs storage)
	KasusTenantProof    = "Zahra Aulia"       // 8: sent a transfer proof from the payment page (needs storage)
	KasusStaffProof     = "Bima Prasetya"     // 9: Yoga sent a transfer proof for them (needs storage)
	KasusOverdue        = "Agus Salim"        // 10: h7 visit; also the tenant of the infected upload
	KasusBroken         = "Citra Dewanti"     // 10: two broken promises
	KasusUndeliverable  = "Nanda Saputri"     // 10: WhatsApp cannot reach them
	KasusPromiseActive  = "Eka Wijaya"        // 11: active promise, due in 3 days
	KasusPromiseKept    = "Fikri Ramadhan"    // 11: promise kept today
	KasusPromisePartly  = "Gita Permata"      // 11: promise partly kept
	KasusVisitDone      = "Irfan Maulana"     // 12: Yoga recorded a promise outcome today
	KasusVisitNotHome   = "Joko Santoso"      // 12: Yoga recorded not home today; still open
	KasusNumberChanged  = "Kevin Saputra"     // 15: Yoga brought back a new number
	KasusTwoLeases      = "Laras Ayuningtyas" // 13: a room and a house
	KasusEndedLease     = "Dedi Kurniawan"    // 13: moved out of Dina's house owing; now in Yoga's room
	KasusQRISMismatch   = "Mega Lestari"      // 15: QRIS paid at another amount
	KasusDepositExact   = "Oki Firmansyah"    // 14: cash in a deposit counted exactly (needs storage)
	KasusDepositGap     = "Putri Handayani"   // 14: cash in a deposit counted short (needs storage)
	KasusOtherKaryawan  = "Xena Anggraini"    // 13: Dina's; Yoga is refused
	kasusNewNumber      = "0857-1100-2233"    // Kevin's new number
	kasusEndedLeaseUnit = "Rumah 2"           // Dedi's ended lease, in the kontrakan
)

// kasusOrg is the case organization. Each lease starts on a day that puts its billing date where
// its case needs it, counted from today: anniversary(n) is a start whose monthly billing date
// falls n days from today, at least 55 days back so it is entered late like the other demo leases.
func kasusOrg(c clock.Clock) orgSpec {
	today := tenant.Today(c)
	anniversary := func(days int) time.Time { return startBilledOn(today.AddDate(0, 0, days), today.AddDate(0, 0, -55)) }
	room := func(name, phone, unit string, billedInDays int, collector string) tenantSpec {
		return tenantSpec{Name: name, Phone: phone, Leases: []leaseSpec{
			{Property: propKasusKos, Unit: unit, Start: anniversary(billedInDays), Collector: collector},
		}}
	}
	// Owing tenants bill again in two weeks, so what they owe today is all overdue.
	const later = 14

	tenants := []tenantSpec{
		room(KasusDueSoon, "0813-0011-0001", "Kamar 1", 2, phoneYoga),
		room(KasusNotDue, "0813-0011-0002", "Kamar 2", 5, phoneYoga),
		room(KasusQRISFull, "0813-0011-0003", "Kamar 3", 12, phoneYoga),
		room(KasusQRISPart, "0813-0011-0004", "Kamar 4", later, phoneYoga),
		room(KasusCredit, "0813-0011-0005", "Kamar 5", 2, phoneYoga),
		room(KasusCashToday, "0813-0011-0006", "Kamar 6", 12, phoneYoga),
		room(KasusCashOther, "0813-0011-0007", "Kamar 7", 12, phoneDina),
		room(KasusTenantProof, "0813-0011-0008", "Kamar 8", later, phoneYoga),
		room(KasusStaffProof, "0813-0011-0009", "Kamar 9", later, phoneYoga),
		room(KasusOverdue, "0813-0011-0010", "Kamar 10", later, phoneYoga),
		room(KasusBroken, "0813-0011-0011", "Kamar 11", later, phoneYoga),
		room(KasusUndeliverable, "0813-0011-0012", "Kamar 12", later, phoneYoga),
		room(KasusPromiseActive, "0813-0011-0013", "Kamar 13", later, phoneYoga),
		room(KasusPromiseKept, "0813-0011-0014", "Kamar 14", 12, phoneYoga),
		room(KasusPromisePartly, "0813-0011-0015", "Kamar 15", later, phoneYoga),
		room(KasusVisitDone, "0813-0011-0016", "Kamar 16", later, phoneYoga),
		room(KasusVisitNotHome, "0813-0011-0017", "Kamar 17", later, phoneYoga),
		room(KasusNumberChanged, "0813-0011-0018", "Kamar 18", later, phoneYoga),
		{Name: KasusTwoLeases, Phone: "0813-0011-0019", Leases: []leaseSpec{
			{Property: propKasusKos, Unit: "Kamar 19", Start: anniversary(later), Collector: phoneYoga},
			{Property: propKasusKontrakan, Unit: "Rumah 1", Start: anniversary(20), Collector: phoneYoga},
		}},
		{Name: KasusEndedLease, Phone: "0813-0011-0020", Leases: []leaseSpec{
			// Billed first, then moved out by SeedCases six days ago, owing; the room is a booking
			// at entry that started five days ago.
			{
				Property: propKasusKontrakan, Unit: kasusEndedLeaseUnit, Start: anniversary(10), Collector: phoneDina,
				EndOn: today.AddDate(0, 0, -6),
			},
			{Property: propKasusKos, Unit: "Kamar 20", Start: today.AddDate(0, 0, -5), Collector: phoneYoga},
		}},
		room(KasusQRISMismatch, "0813-0011-0021", "Kamar 21", later, phoneYoga),
		room(KasusDepositExact, "0813-0011-0022", "Kamar 22", later, phoneYoga),
		room(KasusDepositGap, "0813-0011-0023", "Kamar 23", later, phoneYoga),
		room(KasusOtherKaryawan, "0813-0011-0024", "Kamar 24", later, phoneDina),
	}

	return orgSpec{
		Name:  OrgKasus,
		Actor: phoneKasusAdmin,
		Cases: true,
		BankAccount: &paylink.BankAccount{
			BankName: "BRI", AccountNumber: "0021 0100 5566 507", AccountHolder: "Hadi Santoso",
		},
		QRIS: true,
		Founders: []staffSpec{
			{Phone: phoneKasusSupervisor, Name: "Bu Kartika Sari", Grants: orgWide(authz.RoleSupervisor)},
			{Phone: phoneKasusAdmin, Name: "Pak Hadi Santoso", Grants: orgWide(authz.RoleAdmin)},
		},
		Invited: []staffSpec{
			{Phone: phoneYoga, Name: "Mas Yoga Pratama", Grants: orgWide(authz.RoleKaryawan)},
			{Phone: phoneDina, Name: "Mbak Dina Maharani", Grants: orgWide(authz.RoleKaryawan)},
		},
		Properties: []propertySpec{
			{
				Name: propKasusKos, Kind: property.KindKos, Address: "Jl. Kaliurang Km 9 No. 4, Sleman",
				Units: units("Kamar", 1, 24), Terms: monthly(kasusRoomRent),
			},
			{
				Name: propKasusKontrakan, Kind: property.KindKontrakan, Address: "Jl. Pogung Baru No. 12, Sleman",
				Units: units("Rumah", 1, 2), Terms: monthly(kasusHouseRent),
			},
		},
		Tenants:        tenants,
		BrokenPromises: []string{KasusBroken},
	}
}

// startBilledOn is the latest day on or before notAfter that starts a monthly lease billed on
// target: target itself is one of its period starts. A start on the 29th to 31st bills on the last
// day of shorter months, so a target late in a month may need a start further back; every start
// on the 1st to 28th has one within the month before notAfter's.
func startBilledOn(target, notAfter time.Time) time.Time {
	for d := notAfter; ; d = d.AddDate(0, 0, -1) {
		if tenant.IsPeriodStart(d, 1, target) {
			return d
		}
	}
}
