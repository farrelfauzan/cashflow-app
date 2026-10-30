package demoseed

import (
	"strconv"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

// The demo contents are fixed: the web app's Playwright specs rely on these names, numbers and
// dates. Change them only together with those specs.

// The organization names.
const (
	OrgGriya   = "Griya Sejahtera"
	OrgAnggrek = "Kost Putri Anggrek"
	// OrgBulanan is the big one: enough rows to fill several pages of every list.
	OrgBulanan = "Kos Bulanan Sejahtera"
)

// staffSpec is one staff member. Grants with an empty Property hold for the whole organization.
type staffSpec struct {
	Phone  string
	Name   string
	Grants []grantSpec
	// Disabled members are invited, given their leases, then disabled, so they stay collector
	// on those leases the way a karyawan who leaves does.
	Disabled bool
}

type grantSpec struct {
	Role     string
	Property string // property name; "" means the whole organization
}

type propertySpec struct {
	Name, Kind, Address string
	Units               []string
	// Terms are what every unit offers, with today's prices, unless UnitTerms names the unit.
	Terms     []termSpec
	UnitTerms map[string][]termSpec // unit label -> terms
	// Scheduled are price changes after today, recorded with CreateUnitPrice.
	Scheduled []scheduledSpec
}

// termSpec is one billing term a unit offers and its price for a whole term in force today.
type termSpec struct {
	Months int
	Price  int64 // rupiah
}

// scheduledSpec is a unit's price for a term that takes effect after today.
type scheduledSpec struct {
	Unit   string
	Months int
	Amount int64
	From   time.Time // calendar day
}

// monthly is a unit rented by the month only.
func monthly(price int64) []termSpec { return []termSpec{{Months: 1, Price: price}} }

// termsOf is what the property's unit offers.
func (ps propertySpec) termsOf(label string) []termSpec {
	if t, ok := ps.UnitTerms[label]; ok {
		return t
	}
	return ps.Terms
}

type leaseSpec struct {
	Property, Unit string
	Start          time.Time // calendar day (midnight UTC)
	Term           int       // months, one the unit offers; 0 means 1
	Deposit        int64
	Collector      string    // phone of the collecting member; "" means none
	MovedOut       time.Time // last day of an ended lease; zero for a lease that is still running
	Charges        []chargeSpec
}

// chargeSpec is a fixed monthly extra on a running lease.
type chargeSpec struct {
	Type   string // charge type name
	Amount int64  // rupiah
}

// chargeTypeSpec is a kind of extra charge. A metered one (listrik, air) is billed from usage
// recorded each month, so it is never a fixed lease charge.
type chargeTypeSpec struct {
	Name    string
	Metered bool
}

type tenantSpec struct {
	Name, Phone string
	Leases      []leaseSpec
}

type orgSpec struct {
	Name string
	// Actor is the phone of the organization-wide Admin who makes every change after the
	// organization exists. The organization is found again through their memberships.
	Actor string
	// Founders are created with the organization. Supervisors must be founders: no Admin may
	// grant that role.
	Founders []staffSpec
	// Invited are added by the Actor afterwards, so a property limit can name a property.
	Invited    []staffSpec
	Properties []propertySpec
	// ChargeTypes are the organization's kinds of extra charge.
	ChargeTypes []chargeTypeSpec
	Tenants     []tenantSpec
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// firstOfNextMonth is the first day of next month in Asia/Jakarta, as a calendar day.
func firstOfNextMonth(c clock.Clock) time.Time {
	now := c.Now().In(clock.Jakarta)
	return day(now.Year(), now.Month()+1, 1) // time.Date normalizes December + 1
}

// nextNewYear is the next 1 January in Asia/Jakarta, as a calendar day.
func nextNewYear(c clock.Clock) time.Time {
	return day(c.Now().In(clock.Jakarta).Year()+1, time.January, 1)
}

func orgWide(role string) []grantSpec { return []grantSpec{{Role: role}} }

func units(prefix string, from, to int) []string {
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, prefix+" "+strconv.Itoa(i))
	}
	return out
}

// Shared people.
const (
	phoneHendra = "0812-0009-0001"
	phoneRina   = "0813-0001-0001"

	propKosMelati = "Kos Melati"
	propMawar     = "Kontrakan Mawar"
	propRuko      = "Ruko Godean"
	propAnggrek   = "Kos Putri Anggrek"

	chargeWiFi       = "Wi-Fi"
	chargeParkir     = "Parkir"
	chargeKebersihan = "Kebersihan"
)

func demoOrgs(c clock.Clock) []orgSpec {
	const (
		dimas = "0812-0001-0003"
		joko  = "0812-0001-0005"
		agus  = "0812-0001-0006"
		rudi  = "0812-0002-0003"
	)
	return []orgSpec{
		{
			Name:  OrgGriya,
			Actor: "0812-0001-0002",
			Founders: []staffSpec{
				{Phone: "0812-0001-0001", Name: "Bu Ani Rahmawati", Grants: orgWide(authz.RoleSupervisor)},
				{Phone: "0812-0001-0002", Name: "Pak Budi Hartono", Grants: orgWide(authz.RoleAdmin)},
			},
			Invited: []staffSpec{
				{Phone: dimas, Name: "Mas Dimas Saputra", Grants: orgWide(authz.RoleKaryawan)},
				{Phone: "0812-0001-0004", Name: "Mbak Sari Wulandari", Grants: []grantSpec{
					{Role: authz.RoleAdmin, Property: propKosMelati},
				}},
				{Phone: joko, Name: "Mas Joko Prasetyo", Grants: []grantSpec{
					{Role: authz.RoleKaryawan},
					{Role: authz.RoleAdmin, Property: propMawar},
				}},
				{Phone: phoneHendra, Name: "Pak Hendra Gunawan", Grants: orgWide(authz.RoleAdmin)},
				// A karyawan who left: disabled, still the collector on Eko's lease until an admin
				// picks another ("1 penyewa belum punya penagih").
				{Phone: agus, Name: "Mas Agus Setiawan", Grants: orgWide(authz.RoleKaryawan), Disabled: true},
			},
			Properties: []propertySpec{
				{
					Name: propKosMelati, Kind: property.KindKos, Address: "Jl. Kaliurang Km 5 No. 12, Sleman, Yogyakarta",
					Units: units("Kamar", 1, 6), Terms: monthly(850_000),
					UnitTerms: map[string][]termSpec{
						// Kamar 1 is also let by the year, and its monthly price rises in January.
						"Kamar 1": {{Months: 1, Price: 850_000}, {Months: 12, Price: 9_000_000}},
						"Kamar 2": monthly(900_000),
					},
					Scheduled: []scheduledSpec{{Unit: "Kamar 1", Months: 1, Amount: 900_000, From: nextNewYear(c)}},
				},
				{
					Name: propMawar, Kind: property.KindKontrakan, Address: "Jl. Godean Km 3 No. 7, Yogyakarta",
					Units: []string{"Rumah A", "Rumah B"}, Terms: monthly(1_500_000),
				},
				{
					Name: propRuko, Kind: property.KindRuko, Address: "Jl. Godean Km 6 No. 88, Sleman",
					// Ruko are let by the year only.
					Units: units("Ruko", 1, 2), Terms: []termSpec{{Months: 12, Price: 50_000_000}},
				},
			},
			ChargeTypes: []chargeTypeSpec{
				{Name: "Listrik", Metered: true}, {Name: "Air", Metered: true},
				{Name: chargeWiFi}, {Name: chargeParkir}, {Name: chargeKebersihan},
			},
			Tenants: []tenantSpec{
				{Name: "Rina Wulandari", Phone: phoneRina, Leases: []leaseSpec{
					{
						Property: propKosMelati, Unit: "Kamar 1", Start: day(2026, 7, 1), Deposit: 850_000, Collector: dimas,
						Charges: []chargeSpec{{Type: chargeWiFi, Amount: 50_000}},
					},
					{Property: propMawar, Unit: "Rumah B", Start: day(2026, 8, 1)},
				}},
				{Name: "Budi Santoso", Phone: "0813-0001-0002", Leases: []leaseSpec{
					{
						Property: propKosMelati, Unit: "Kamar 2", Start: day(2026, 9, 1), Collector: dimas,
						Charges: []chargeSpec{{Type: chargeWiFi, Amount: 50_000}},
					},
				}},
				{Name: "Citra Lestari", Phone: "0813-0001-0003", Leases: []leaseSpec{
					{Property: propKosMelati, Unit: "Kamar 3", Start: day(2026, 8, 15), Collector: joko},
				}},
				{Name: "Dewi Anggraini", Phone: "0813-0001-0004", Leases: []leaseSpec{
					// The spec fixes only the move-out; the start and terms are the unit's usual ones.
					{
						Property: propKosMelati, Unit: "Kamar 4", Start: day(2026, 4, 1),
						MovedOut: day(2026, 9, 30),
					},
				}},
				{Name: "Eko Prasetyo", Phone: "0813-0001-0005", Leases: []leaseSpec{
					{Property: propMawar, Unit: "Rumah A", Start: day(2026, 6, 1), Collector: agus},
				}},
				{Name: "Toko Sumber Rejeki (Pak Agus)", Phone: "0813-0001-0006", Leases: []leaseSpec{
					{
						Property: propRuko, Unit: "Ruko 1", Start: day(2026, 1, 1), Term: 12, Collector: joko,
						Charges: []chargeSpec{{Type: chargeParkir, Amount: 100_000}, {Type: chargeKebersihan, Amount: 50_000}},
					},
				}},
				{Name: "Fajar Nugroho", Phone: "0813-0001-0007", Leases: []leaseSpec{
					// A booking: it starts on the first day of next month.
					{Property: propKosMelati, Unit: "Kamar 5", Start: firstOfNextMonth(c)},
				}},
			},
		},
		{
			Name:  OrgAnggrek,
			Actor: "0812-0002-0002",
			Founders: []staffSpec{
				{Phone: "0812-0002-0001", Name: "Ibu Lestari Handayani", Grants: orgWide(authz.RoleSupervisor)},
				{Phone: "0812-0002-0002", Name: "Mbak Wulan Sari", Grants: orgWide(authz.RoleAdmin)},
				{Phone: phoneHendra, Name: "Pak Hendra Gunawan", Grants: orgWide(authz.RoleSupervisor)},
			},
			Invited: []staffSpec{
				{Phone: rudi, Name: "Mas Rudi Hartanto", Grants: orgWide(authz.RoleKaryawan)},
			},
			Properties: []propertySpec{
				{
					Name: propAnggrek, Kind: property.KindKos, Address: "Jl. Gejayan No. 21, Sleman",
					Units: units("Kamar", 1, 4), Terms: monthly(750_000),
					UnitTerms: map[string][]termSpec{"Kamar 3": monthly(800_000)},
				},
			},
			Tenants: []tenantSpec{
				{Name: "Ayu Lestari", Phone: "0813-0002-0001", Leases: []leaseSpec{
					{Property: propAnggrek, Unit: "Kamar 1", Start: day(2026, 8, 1), Collector: rudi},
				}},
				{Name: "Nadia Putri", Phone: "0813-0002-0002", Leases: []leaseSpec{
					{Property: propAnggrek, Unit: "Kamar 2", Start: day(2026, 9, 1)},
				}},
				// The same phone as Rina in Griya Sejahtera: a separate tenant record here.
				{Name: "Rina Wulandari", Phone: phoneRina, Leases: []leaseSpec{
					{Property: propAnggrek, Unit: "Kamar 3", Start: day(2026, 10, 1)},
				}},
			},
		},
		bulananOrg(),
	}
}
