package config

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// ForgeExtensionKey is the top-level config key holding the self-hosted forge
// block:
//
//	[forge]
//	url = "https://forge.example.com"
//	remote_name = "forge"
//	token_command = "op read op://grove/forge/token"
//
// It is an extension namespace rather than a core Config field, so it parses
// through the existing Extensions path with no change to the config loader,
// the schema, or the serialization round-trip.
const ForgeExtensionKey = "forge"

// DefaultForgeRemoteName is the git remote name a forge is enrolled under when
// [forge] does not say otherwise. It is deliberately not "origin": a forge
// remote sits alongside the upstream, it does not replace it.
const DefaultForgeRemoteName = "forge"

// ForgeConfig is the typed [forge] block.
//
// The instance fields (URL, RemoteName, TokenCommand) are no longer inert: the
// GLOBAL DAEMON reads them to construct a forgejo provider and resolves
// TokenCommand itself (see ForgePollConfig and the daemon's forge poller).
// Token custody stays daemon-side by construction — core/pkg/forge accepts an
// already-resolved token and never shells out for one, and no CLI or TUI in the
// ecosystem executes TokenCommand. This struct still only carries the string.
//
// Poll is the daemon forge poller's opt-in. It sits here because it configures
// forge integration, not because it configures the instance above it: with no
// URL at all the poller falls back to GitHub-via-`gh` over repo origins.
//
// Infra and Services configure `grove forge up` — the services VM that HOSTS
// the instance URL points at. They are CLI-side inputs the daemon never reads,
// nested under [forge] rather than parked in a sibling top-level key because
// every other host-lifecycle block in grove nests under the noun it belongs to
// ([satellites.<name>.infra], [satellites.<name>.provision]).
type ForgeConfig struct {
	// URL is the base URL of the self-hosted forge instance.
	URL string `yaml:"url,omitempty" toml:"url,omitempty" jsonschema:"description=Base URL of the self-hosted forge instance"`
	// RemoteName is the git remote the forge is enrolled under. Empty means
	// DefaultForgeRemoteName; see EffectiveRemoteName.
	RemoteName string `yaml:"remote_name,omitempty" toml:"remote_name,omitempty" jsonschema:"description=Git remote name for the forge,default=forge"`
	// TokenCommand is a shell command whose trimmed stdout is the forge API
	// token. It is stored here, and executed ONLY by the global daemon.
	TokenCommand string `yaml:"token_command,omitempty" toml:"token_command,omitempty" jsonschema:"description=Shell command that prints the forge API token (executed by the global daemon only)" jsonschema_extras:"x-sensitive=true,x-hint=Prefer token_command over embedding a token in config"`
	// Provider selects which forge provider the daemon poller constructs. See
	// the ForgeProvider* constants; empty means ForgeProviderAuto.
	Provider string `yaml:"provider,omitempty" toml:"provider,omitempty" jsonschema:"description=Forge provider the daemon poller uses: auto|forgejo|github,default=auto"`
	// Poll is the [forge.poll] sub-block configuring the daemon's read-only
	// forge poller. Absent means the poller stays off.
	Poll *ForgePollConfig `yaml:"poll,omitempty" toml:"poll,omitempty" jsonschema:"description=Daemon read-only forge poller (PR + checks state)"`
	// Infra is the [forge.infra] sub-block: terraform inputs for the services
	// VM `grove forge up` provisions.
	Infra *ForgeInfraConfig `yaml:"infra,omitempty" toml:"infra,omitempty" jsonschema:"description=Terraform inputs for the grove forge services VM"`
	// Services is the [forge.services] sub-block: what the services VM runs
	// (Forgejo, grove-syncd) and how it terminates TLS.
	Services *ForgeServicesConfig `yaml:"services,omitempty" toml:"services,omitempty" jsonschema:"description=Services colocated on the grove forge VM"`
	// Backup is the [forge.backup] sub-block: the off-VM GCS backup of both
	// services' durable state. Absent means no bucket, no timer, no scopes —
	// the forge stays a machine with no GCP API access at all.
	Backup *ForgeBackupConfig `yaml:"backup,omitempty" toml:"backup,omitempty" jsonschema:"description=Off-VM GCS backup of the forge's durable state"`
	// Wireguard is the optional [forge.wireguard] client block. It is
	// converged over pinned SSH after provisioning; terraform never sees it.
	Wireguard *WireGuardConfig `yaml:"wireguard,omitempty" toml:"wireguard,omitempty" jsonschema:"description=Owner-managed WireGuard mesh client (private key remains on the VM)"`
}

// Forge provider selectors for [forge] provider.
//
// The default is deliberately derived rather than fixed: a machine that has
// declared a [forge] url wants that instance polled, and one that has not still
// wants the GitHub-over-origin behavior the poller shipped with. Naming a
// provider explicitly is the escape hatch for the mixed case (a forge VM
// configured for hosting, but PRs still watched on GitHub).
const (
	// ForgeProviderAuto picks forgejo when a URL is configured, else github.
	ForgeProviderAuto = "auto"
	// ForgeProviderForgejo forces the Forgejo/Gitea REST provider. It requires
	// a URL.
	ForgeProviderForgejo = "forgejo"
	// ForgeProviderGitHub forces the GitHub-via-`gh` provider, ignoring URL.
	ForgeProviderGitHub = "github"
)

// Poller cadence bounds. The floor exists so a typo cannot turn the daemon into
// a request generator against someone else's API; the defaults are what a
// laptop watching its own ecosystem wants.
const (
	// DefaultForgePollInterval is the gap between poll sweeps when
	// [forge.poll] does not say otherwise.
	DefaultForgePollInterval = 5 * time.Minute
	// MinForgePollInterval is the floor a configured interval is clamped to.
	MinForgePollInterval = 1 * time.Minute
	// DefaultForgeStaleAfter is how long a successful fetch stays "fresh"
	// before the poller degrades it to stale of its own accord.
	DefaultForgeStaleAfter = 15 * time.Minute
	// MinForgeStaleAfter is the floor for StaleAfter. A window shorter than the
	// poll interval would mark every entry stale between sweeps.
	MinForgeStaleAfter = 2 * time.Minute
)

// ForgePollConfig is the typed [forge.poll] block:
//
//	[forge.poll]
//	enabled = true
//	interval = "5m"
//	stale_after = "15m"
//
// It is OFF by default and stays off unless Enabled is explicitly true. The
// poller additionally refuses to run when its provider reports the transport
// unavailable, so enabling this on a machine with no `gh` is a no-op with a
// log line rather than an error.
type ForgePollConfig struct {
	// Enabled is the explicit opt-in. False (and absent) means no poller, no
	// goroutine, no network reads.
	Enabled bool `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Enable the daemon forge poller,default=false"`
	// Interval is a Go duration string ("5m") for the gap between sweeps.
	// Empty means DefaultForgePollInterval; anything below
	// MinForgePollInterval is clamped up. See EffectiveInterval.
	Interval string `yaml:"interval,omitempty" toml:"interval,omitempty" jsonschema:"description=Gap between poll sweeps (Go duration),default=5m"`
	// StaleAfter is a Go duration string for how long fetched data stays
	// fresh. See EffectiveStaleAfter.
	StaleAfter string `yaml:"stale_after,omitempty" toml:"stale_after,omitempty" jsonschema:"description=How long fetched forge data stays fresh (Go duration),default=15m"`
}

// PollEnabled reports whether the daemon forge poller is opted in. Nil-safe at
// both levels, because "no [forge] block at all" is the normal state.
func (f *ForgeConfig) PollEnabled() bool {
	return f != nil && f.Poll != nil && f.Poll.Enabled
}

// EffectiveInterval resolves Interval against the default and the floor. An
// unparseable duration falls back to the default rather than failing: the
// poller is an optional read-only convenience and must never be a reason a
// daemon does not boot.
func (p *ForgePollConfig) EffectiveInterval() time.Duration {
	return effectiveForgeDuration(p.rawInterval(), DefaultForgePollInterval, MinForgePollInterval)
}

// EffectiveStaleAfter resolves StaleAfter against the default and the floor,
// then guarantees it is at least one interval long — a freshness window
// narrower than the sweep gap would report every entry stale between sweeps.
func (p *ForgePollConfig) EffectiveStaleAfter() time.Duration {
	stale := effectiveForgeDuration(p.rawStaleAfter(), DefaultForgeStaleAfter, MinForgeStaleAfter)
	if interval := p.EffectiveInterval(); stale < interval {
		return interval
	}
	return stale
}

func (p *ForgePollConfig) rawInterval() string {
	if p == nil {
		return ""
	}
	return p.Interval
}

func (p *ForgePollConfig) rawStaleAfter() string {
	if p == nil {
		return ""
	}
	return p.StaleAfter
}

func effectiveForgeDuration(raw string, def, floor time.Duration) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return def
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil || d <= 0 {
		return def
	}
	if d < floor {
		return floor
	}
	return d
}

// EffectiveRemoteName resolves RemoteName against the default.
func (f *ForgeConfig) EffectiveRemoteName() string {
	if f == nil || strings.TrimSpace(f.RemoteName) == "" {
		return DefaultForgeRemoteName
	}
	return strings.TrimSpace(f.RemoteName)
}

// EffectiveProvider resolves Provider, including the "auto" derivation: a
// configured URL means the self-hosted instance is the thing worth polling.
// An unrecognized value resolves to auto rather than erroring — Validate is
// where a typo is reported, and an optional read-only poller must never be a
// reason the daemon does not boot.
func (f *ForgeConfig) EffectiveProvider() string {
	raw := ""
	if f != nil {
		raw = strings.ToLower(strings.TrimSpace(f.Provider))
	}
	switch raw {
	case ForgeProviderForgejo, ForgeProviderGitHub:
		return raw
	}
	if f.IsConfigured() {
		return ForgeProviderForgejo
	}
	return ForgeProviderGitHub
}

// Host is the hostname of the configured forge URL, lowercased — the value a
// caller compares a git remote's host against. Empty when no URL is set or it
// does not parse.
func (f *ForgeConfig) Host() string {
	if f == nil {
		return ""
	}
	raw := strings.TrimSpace(f.URL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// ---- [forge.infra] --------------------------------------------------------

// Defaults for the services VM. e2-medium rather than the trial's e2-small:
// two services (Forgejo's Go binary plus grove-syncd) on 2 GB was tight, and
// the difference is a few dollars a month on a machine that is a pet.
const (
	// DefaultForgeVMName is the instance name (also the network tag and the
	// firewall-rule prefix) when [forge.infra] does not say otherwise.
	DefaultForgeVMName = "grove-forge"
	// DefaultForgeMachineType is the GCE machine type for the services VM.
	DefaultForgeMachineType = "e2-medium"
	// DefaultForgeZone matches the satellite module's default zone.
	DefaultForgeZone = "us-east1-b"
	// DefaultForgeDiskSizeGB sizes the boot disk. The forge accumulates
	// durable refs, so this is the one dimension worth over-provisioning.
	DefaultForgeDiskSizeGB = 50
	// DefaultForgeImageFamily/Project pin Debian 12 (the concept's choice; the
	// satellite module's Ubuntu is a build host, this is a service host).
	DefaultForgeImageFamily  = "debian-12"
	DefaultForgeImageProject = "debian-cloud"
)

// ForgeInfraConfig is the [forge.infra] block: the terraform inputs for the
// services VM. It mirrors [satellites.<name>.infra] key-for-key where the two
// overlap, so an operator who has provisioned a satellite already knows this
// block — but it is a SEPARATE block under a separate noun, because the forge
// is a pet and the satellite verbs are cattle verbs.
type ForgeInfraConfig struct {
	// Project is the GCP project id (terraform var project_id). Required.
	Project string `yaml:"project,omitempty" toml:"project,omitempty" jsonschema:"description=GCP project id for the forge VM"`
	// Zone is the GCP zone (terraform var zone).
	Zone string `yaml:"zone,omitempty" toml:"zone,omitempty" jsonschema:"description=GCP zone for the forge VM,default=us-east1-b"`
	// VMName is the instance name, network tag and firewall-rule prefix.
	VMName string `yaml:"vm_name,omitempty" toml:"vm_name,omitempty" jsonschema:"description=Forge VM instance name,default=grove-forge"`
	// MachineType is the GCE machine type.
	MachineType string `yaml:"machine_type,omitempty" toml:"machine_type,omitempty" jsonschema:"description=GCE machine type,default=e2-medium"`
	// DiskSizeGB sizes the boot disk (which holds every git ref the forge ever
	// accepts, plus its SQLite database).
	DiskSizeGB int `yaml:"disk_size_gb,omitempty" toml:"disk_size_gb,omitempty" jsonschema:"description=Boot disk size in GB,default=50"`
	// ImageFamily/ImageProject select the OS image.
	ImageFamily  string `yaml:"image_family,omitempty" toml:"image_family,omitempty" jsonschema:"description=OS image family,default=debian-12"`
	ImageProject string `yaml:"image_project,omitempty" toml:"image_project,omitempty" jsonschema:"description=Project owning the OS image,default=debian-cloud"`
	// SSHUser is the login user provisioned via instance metadata. Required.
	SSHUser string `yaml:"ssh_user,omitempty" toml:"ssh_user,omitempty" jsonschema:"description=SSH login user on the forge VM"`
	// SSHPubkeyFile is the public key authorized for SSHUser.
	SSHPubkeyFile string `yaml:"ssh_pubkey_file,omitempty" toml:"ssh_pubkey_file,omitempty" jsonschema:"description=SSH public key granted access,default=~/.ssh/id_ed25519.pub"`
	// IdentityFile is the private key `grove forge` uses to reach the VM.
	IdentityFile string `yaml:"identity_file,omitempty" toml:"identity_file,omitempty" jsonschema:"description=SSH private key used to reach the forge VM"`
	// CIDR is the operator CIDR allowed to reach tcp/22 and the service ports.
	// 0.0.0.0/0 is refused, here and in the terraform module.
	CIDR string `yaml:"cidr,omitempty" toml:"cidr,omitempty" jsonschema:"description=Operator CIDR allowed to reach SSH and the service ports (never 0.0.0.0/0)"`
	// ServiceAccountEmail attaches an EXISTING service account. Empty is the
	// recommended value: the module then creates a dedicated one with no IAM
	// roles and no OAuth scopes, closing the trial's finding that the default
	// compute SA can read every bucket in the project.
	ServiceAccountEmail string `yaml:"service_account_email,omitempty" toml:"service_account_email,omitempty" jsonschema:"description=Existing service account to attach; empty means create a dedicated no-scope one"`
	// EnableIAPSSH adds the IAP TCP-forwarding range (35.235.240.0/20) to the
	// SSH rule, so SSH survives a laptop IP rotation. Nil means enabled.
	EnableIAPSSH *bool `yaml:"enable_iap_ssh,omitempty" toml:"enable_iap_ssh,omitempty" jsonschema:"description=Allow IAP TCP forwarding to tcp/22,default=true"`
}

// EffectiveZone and friends resolve one field each against its default. They
// are nil-safe: an absent [forge.infra] block still yields a usable plan for
// everything that HAS a default, and the required fields are reported by
// Validate rather than defaulted to something wrong.
func (i *ForgeInfraConfig) EffectiveZone() string {
	return forgeStringDefault(i.rawZone(), DefaultForgeZone)
}

func (i *ForgeInfraConfig) EffectiveVMName() string {
	return forgeStringDefault(i.rawVMName(), DefaultForgeVMName)
}

func (i *ForgeInfraConfig) EffectiveMachineType() string {
	return forgeStringDefault(i.rawMachineType(), DefaultForgeMachineType)
}

func (i *ForgeInfraConfig) EffectiveImageFamily() string {
	return forgeStringDefault(i.rawImageFamily(), DefaultForgeImageFamily)
}

func (i *ForgeInfraConfig) EffectiveImageProject() string {
	return forgeStringDefault(i.rawImageProject(), DefaultForgeImageProject)
}

// EffectiveDiskSizeGB clamps a nonsensical (zero or negative) size up to the
// default rather than shipping it to terraform.
func (i *ForgeInfraConfig) EffectiveDiskSizeGB() int {
	if i == nil || i.DiskSizeGB <= 0 {
		return DefaultForgeDiskSizeGB
	}
	return i.DiskSizeGB
}

// IAPSSHEnabled reports whether the IAP range joins the SSH rule. Absent means
// enabled: the alternative to IAP is pinning a laptop IP that rotates, and the
// trial's hardening ledger already had to un-wedge exactly that.
func (i *ForgeInfraConfig) IAPSSHEnabled() bool {
	if i == nil || i.EnableIAPSSH == nil {
		return true
	}
	return *i.EnableIAPSSH
}

func (i *ForgeInfraConfig) rawZone() string {
	if i == nil {
		return ""
	}
	return i.Zone
}

func (i *ForgeInfraConfig) rawVMName() string {
	if i == nil {
		return ""
	}
	return i.VMName
}

func (i *ForgeInfraConfig) rawMachineType() string {
	if i == nil {
		return ""
	}
	return i.MachineType
}

func (i *ForgeInfraConfig) rawImageFamily() string {
	if i == nil {
		return ""
	}
	return i.ImageFamily
}

func (i *ForgeInfraConfig) rawImageProject() string {
	if i == nil {
		return ""
	}
	return i.ImageProject
}

// ---- [forge.services] -----------------------------------------------------

// TLS modes for the services VM.
const (
	// ForgeTLSSelfSigned generates a certificate on the VM at first boot and
	// pins it by fingerprint (surfaced by `grove forge status`). It is the
	// default because it needs no DNS, no registrar, and — unlike an ACME
	// HTTP-01 challenge — no 0.0.0.0/0 ingress on :80.
	ForgeTLSSelfSigned = "self-signed"
	// ForgeTLSACME obtains a real certificate via ACME DNS-01. HTTP-01 is
	// deliberately not offered: it would require opening :80 to the world,
	// which this module refuses to do for any port.
	ForgeTLSACME = "acme"
)

// Service defaults.
const (
	// DefaultForgejoHTTPPort is Forgejo's HTTP port (its own default).
	DefaultForgejoHTTPPort = 3000
	// DefaultForgeSyncdPort is the grove-syncd bind port, matching the
	// satellite sync contract's remote default (127.0.0.1:8788).
	DefaultForgeSyncdPort = 8788
)

// ForgeServicesConfig is the [forge.services] block: what runs on the VM.
//
// Forgejo and grove-syncd are colocated deliberately (forge-hosting.md): same
// TLS discipline, same token discipline, one box to administer and back up.
type ForgeServicesConfig struct {
	// Domain is the DNS name the services are reached at. Empty means the
	// external IP is the only address, which forces ForgeTLSSelfSigned.
	Domain string `yaml:"domain,omitempty" toml:"domain,omitempty" jsonschema:"description=DNS name of the forge services VM"`
	// TLSMode is ForgeTLSSelfSigned (default) or ForgeTLSACME.
	TLSMode string `yaml:"tls_mode,omitempty" toml:"tls_mode,omitempty" jsonschema:"description=TLS strategy: self-signed|acme,default=self-signed"`
	// ACMEEmail is the registration contact for ForgeTLSACME.
	ACMEEmail string `yaml:"acme_email,omitempty" toml:"acme_email,omitempty" jsonschema:"description=ACME account email (tls_mode = acme)"`
	// ACMEDNSProvider is the lego DNS-01 provider code (e.g. "cloudflare",
	// "gcloud"). Required for ForgeTLSACME — see the TLS mode comment for why
	// DNS-01 is the only challenge offered.
	ACMEDNSProvider string `yaml:"acme_dns_provider,omitempty" toml:"acme_dns_provider,omitempty" jsonschema:"description=lego DNS-01 provider code (tls_mode = acme)"`
	// Forgejo configures the Forgejo service.
	Forgejo *ForgejoServiceConfig `yaml:"forgejo,omitempty" toml:"forgejo,omitempty" jsonschema:"description=Forgejo service on the forge VM"`
	// Syncd configures the colocated grove-syncd service.
	Syncd *ForgeSyncdServiceConfig `yaml:"syncd,omitempty" toml:"syncd,omitempty" jsonschema:"description=grove-syncd service on the forge VM"`
}

// ForgejoServiceConfig is [forge.services.forgejo].
//
// Registration-off and INSTALL_LOCK are NOT configurable: an open forge is a
// posture change, not a preference, and the trial already proved the headless
// install path works with both nailed down.
type ForgejoServiceConfig struct {
	// Version is the Forgejo release to install (e.g. "16.0.2"). Required to
	// provision: an unpinned forge is one upstream release away from a
	// surprise.
	Version string `yaml:"version,omitempty" toml:"version,omitempty" jsonschema:"description=Forgejo release version to install"`
	// SHA256 is the hex checksum of the release binary. Required alongside
	// Version — a pinned version with an unverified download is not a pin.
	SHA256 string `yaml:"sha256,omitempty" toml:"sha256,omitempty" jsonschema:"description=SHA-256 of the Forgejo release binary"`
	// HTTPPort is the port Forgejo listens on. Exposure stays limited to the
	// operator CIDR regardless; see the module's firewall rules.
	HTTPPort int `yaml:"http_port,omitempty" toml:"http_port,omitempty" jsonschema:"description=Forgejo HTTP port,default=3000"`
	// SiteName is Forgejo's APP_NAME.
	SiteName string `yaml:"site_name,omitempty" toml:"site_name,omitempty" jsonschema:"description=Forgejo site name,default=grove forge"`
}

// EffectiveHTTPPort resolves HTTPPort against Forgejo's own default.
func (f *ForgejoServiceConfig) EffectiveHTTPPort() int {
	if f == nil || f.HTTPPort <= 0 {
		return DefaultForgejoHTTPPort
	}
	return f.HTTPPort
}

// EffectiveSiteName resolves SiteName.
func (f *ForgejoServiceConfig) EffectiveSiteName() string {
	if f == nil || strings.TrimSpace(f.SiteName) == "" {
		return "grove forge"
	}
	return strings.TrimSpace(f.SiteName)
}

// Forge backup defaults. The retention numbers are deliberately generous for
// data measured in megabytes: a personal notebook corpus plus a handful of git
// repos is cheap to keep, and the expensive mistake is keeping too little.
const (
	// DefaultForgeBackupSchedule is the systemd OnCalendar expression for the
	// backup timer.
	DefaultForgeBackupSchedule = "daily"
	// DefaultForgeBackupRetentionDays deletes objects (current and noncurrent)
	// older than this many days.
	DefaultForgeBackupRetentionDays = 180
	// DefaultForgeBackupNearlineDays moves live objects to Nearline storage
	// after this many days.
	DefaultForgeBackupNearlineDays = 30
	// DefaultForgeBackupNoncurrentDays deletes superseded object versions
	// after this many days.
	DefaultForgeBackupNoncurrentDays = 30
	// DefaultForgeBackupStaleAfter is how old the LAST_SUCCESS marker may get
	// before the staleness check alerts.
	DefaultForgeBackupStaleAfter = "48h"
	// DefaultForgeBackupLocalKeep is how many snapshots stay on the VM's own
	// disk after a successful upload.
	DefaultForgeBackupLocalKeep = 3
	// ForgeBackupOAuthScope is the ONE OAuth scope the backup adds to the
	// forge's service account. It is storage-only and read-write, not
	// cloud-platform: the widening is exactly "may write objects", and the
	// bucket-level IAM binding is what says WHICH bucket.
	ForgeBackupOAuthScope = "https://www.googleapis.com/auth/devstorage.read_write"
)

// ForgeBackupConfig is the [forge.backup] block: an off-VM copy of everything
// the forge's boot disk holds that cannot be rebuilt from somewhere else.
//
// It is OFF by default, and that default is a security property rather than
// laziness. The module's headline posture is a service account with no IAM
// roles attached with no OAuth scopes (main.tf item 3); turning backups on is
// the one deliberate widening of it, and it widens by exactly one scope
// (ForgeBackupOAuthScope) plus one IAM binding scoped to one bucket. A forge
// with no [forge.backup] block still has no GCP API access at all.
//
// What is backed up, and why it is not `forgejo dump`:
//
//   - grove-syncd: `grove-syncd backup` (SQLite VACUUM INTO — consistent while
//     serving) plus a mirror of the content-addressed blob tier.
//   - Forgejo: a VACUUM INTO of its SQLite database plus a tar of the repo
//     tree. Job 17's restore rehearsal found `forgejo dump`'s SQL export is not
//     restorable on the platform this module provisions — it emits `unistr()`
//     literals that need SQLite ≥ 3.51, and debian-12 ships 3.40.1, so the load
//     dies partway and leaves a half-populated database.
//
// What is deliberately NOT backed up: Forgejo's app.ini and the secrets file
// beside it. They hold SECRET_KEY, INTERNAL_TOKEN and the JWT secrets, and an
// artifact carrying them is a credential at rest in a bucket. The cost is
// stated rather than hidden: SECRET_KEY-encrypted material (2FA enrolments,
// stored mirror credentials) does not survive a restore. API tokens and
// password hashes live in the database and do survive.
type ForgeBackupConfig struct {
	// Enabled turns the whole thing on: the bucket, the OAuth scope, the IAM
	// binding, the on-VM script and the timer. Nil/false means none of it
	// exists.
	Enabled *bool `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Provision the GCS backup bucket, scope, timer and script,default=false"`
	// CreateBucket provisions the bucket, rather than joining one that already
	// exists. Nil means true. False is what a REPLACEMENT forge wants: during a
	// restore it needs read access to the outgoing forge's bucket, not
	// ownership of it — and adopting a live bucket into a scratch VM's
	// terraform state is how a drill destroys the thing it was rehearsing.
	CreateBucket *bool `yaml:"create_bucket,omitempty" toml:"create_bucket,omitempty" jsonschema:"description=Create the bucket rather than joining an existing one,default=true"`
	// Bucket is the GCS bucket name (no gs:// prefix). Required when enabled:
	// there is no safe default, because a guessed bucket name is either
	// someone else's or a typo that silently backs up nothing.
	Bucket string `yaml:"bucket,omitempty" toml:"bucket,omitempty" jsonschema:"description=GCS bucket for forge backups (no gs:// prefix)"`
	// Location is the bucket's GCS location. Empty means the region the VM's
	// zone sits in.
	Location string `yaml:"location,omitempty" toml:"location,omitempty" jsonschema:"description=GCS bucket location; empty means the VM's region"`
	// Prefix namespaces objects inside the bucket, so one bucket can hold more
	// than one forge. Empty means the VM name.
	Prefix string `yaml:"prefix,omitempty" toml:"prefix,omitempty" jsonschema:"description=Object prefix inside the bucket; empty means the VM name"`
	// Schedule is the systemd OnCalendar expression for the backup timer.
	Schedule string `yaml:"schedule,omitempty" toml:"schedule,omitempty" jsonschema:"description=systemd OnCalendar expression for the backup timer,default=daily"`
	// RetentionDays deletes objects older than this. NearlineDays moves live
	// objects to Nearline. NoncurrentDays deletes superseded versions.
	RetentionDays  int `yaml:"retention_days,omitempty" toml:"retention_days,omitempty" jsonschema:"description=Delete backup objects older than this many days,default=180"`
	NearlineDays   int `yaml:"nearline_days,omitempty" toml:"nearline_days,omitempty" jsonschema:"description=Move live objects to Nearline after this many days,default=30"`
	NoncurrentDays int `yaml:"noncurrent_days,omitempty" toml:"noncurrent_days,omitempty" jsonschema:"description=Delete superseded object versions after this many days,default=30"`
	// LocalKeep is how many snapshots stay on the VM disk post-upload.
	LocalKeep int `yaml:"local_keep,omitempty" toml:"local_keep,omitempty" jsonschema:"description=Snapshots retained on the VM's own disk,default=3"`
	// StaleAfter is how old LAST_SUCCESS may get before the staleness timer
	// alerts. Staleness is the alerting signal precisely because it catches
	// the failure mode a per-run alert cannot: a timer that stopped firing.
	StaleAfter string `yaml:"stale_after,omitempty" toml:"stale_after,omitempty" jsonschema:"description=Alert when LAST_SUCCESS is older than this,default=48h"`
	// NtfyURL and NtfyTopicCommand configure failure alerting over the ntfy
	// path the ecosystem already uses.
	//
	// The topic is resolved by a COMMAND on the laptop and shipped to the VM
	// over SSH into a 0600 file, exactly like the grove-syncd binary — never
	// through terraform, whose variables and state are readable by anything
	// that can read the state file. A topic in tfvars would be a shared secret
	// sitting in a plan output.
	NtfyURL          string `yaml:"ntfy_url,omitempty" toml:"ntfy_url,omitempty" jsonschema:"description=ntfy base URL for backup failure alerts"`
	NtfyTopicCommand string `yaml:"ntfy_topic_command,omitempty" toml:"ntfy_topic_command,omitempty" jsonschema:"description=Shell command printing the ntfy topic; run on the laptop, shipped to the VM over SSH" jsonschema_extras:"x-sensitive=true"`
}

// BackupEnabled reports whether backups are provisioned (nil-safe, default off).
func (f *ForgeConfig) BackupEnabled() bool {
	if f == nil || f.Backup == nil || f.Backup.Enabled == nil {
		return false
	}
	return *f.Backup.Enabled && strings.TrimSpace(f.Backup.Bucket) != ""
}

// CreateBucketEnabled reports whether terraform owns the bucket (nil-safe,
// default true).
func (b *ForgeBackupConfig) CreateBucketEnabled() bool {
	if b == nil || b.CreateBucket == nil {
		return true
	}
	return *b.CreateBucket
}

// EffectivePrefix namespaces this forge's objects inside the bucket.
func (b *ForgeBackupConfig) EffectivePrefix(vmName string) string {
	if b == nil || strings.TrimSpace(b.Prefix) == "" {
		return vmName
	}
	return strings.Trim(strings.TrimSpace(b.Prefix), "/")
}

// EffectiveLocation resolves the bucket location, defaulting to the region the
// zone sits in ("us-east1-b" → "us-east1").
func (b *ForgeBackupConfig) EffectiveLocation(zone string) string {
	if b != nil && strings.TrimSpace(b.Location) != "" {
		return strings.TrimSpace(b.Location)
	}
	if i := strings.LastIndex(zone, "-"); i > 0 {
		return zone[:i]
	}
	return zone
}

func (b *ForgeBackupConfig) EffectiveSchedule() string {
	if b == nil || strings.TrimSpace(b.Schedule) == "" {
		return DefaultForgeBackupSchedule
	}
	return strings.TrimSpace(b.Schedule)
}

func (b *ForgeBackupConfig) EffectiveRetentionDays() int {
	return forgeBackupIntDefault(b != nil, func() int { return b.RetentionDays }, DefaultForgeBackupRetentionDays)
}

func (b *ForgeBackupConfig) EffectiveNearlineDays() int {
	return forgeBackupIntDefault(b != nil, func() int { return b.NearlineDays }, DefaultForgeBackupNearlineDays)
}

func (b *ForgeBackupConfig) EffectiveNoncurrentDays() int {
	return forgeBackupIntDefault(b != nil, func() int { return b.NoncurrentDays }, DefaultForgeBackupNoncurrentDays)
}

func (b *ForgeBackupConfig) EffectiveLocalKeep() int {
	return forgeBackupIntDefault(b != nil, func() int { return b.LocalKeep }, DefaultForgeBackupLocalKeep)
}

func (b *ForgeBackupConfig) EffectiveStaleAfter() string {
	if b == nil || strings.TrimSpace(b.StaleAfter) == "" {
		return DefaultForgeBackupStaleAfter
	}
	return strings.TrimSpace(b.StaleAfter)
}

func (b *ForgeBackupConfig) EffectiveNtfyURL() string {
	if b == nil || strings.TrimSpace(b.NtfyURL) == "" {
		return "https://ntfy.sh"
	}
	return strings.TrimRight(strings.TrimSpace(b.NtfyURL), "/")
}

func forgeBackupIntDefault(present bool, get func() int, def int) int {
	if !present {
		return def
	}
	if v := get(); v > 0 {
		return v
	}
	return def
}

// ResolveNtfyTopic runs NtfyTopicCommand and returns its trimmed stdout. An
// empty NtfyTopicCommand yields ("", nil): alerting is optional, and a forge
// without it still backs up.
func (b *ForgeBackupConfig) ResolveNtfyTopic() (string, error) {
	if b == nil || strings.TrimSpace(b.NtfyTopicCommand) == "" {
		return "", nil
	}
	cmd := exec.Command("sh", "-c", b.NtfyTopicCommand) //nolint:gosec // command comes from the operator's own grove config
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("forge backup ntfy_topic_command %q failed: %w", b.NtfyTopicCommand, err)
	}
	topic := strings.TrimSpace(string(out))
	if topic == "" {
		return "", fmt.Errorf("forge backup ntfy_topic_command %q printed nothing", b.NtfyTopicCommand)
	}
	return topic, nil
}

// Validate checks the [forge.backup] block's structure.
func (b *ForgeBackupConfig) Validate() error {
	if b == nil {
		return nil
	}
	enabled := b.Enabled != nil && *b.Enabled
	bucket := strings.TrimSpace(b.Bucket)
	if enabled && bucket == "" {
		return fmt.Errorf("[forge.backup] enabled = true needs a bucket (there is no safe default: a guessed bucket name either belongs to someone else or silently backs up nothing)")
	}
	if bucket != "" {
		if strings.Contains(bucket, "://") {
			return fmt.Errorf("forge backup bucket %q must be a bare bucket name, not a URL", b.Bucket)
		}
		if len(bucket) < 3 || len(bucket) > 63 {
			return fmt.Errorf("forge backup bucket %q must be 3-63 characters", b.Bucket)
		}
	}
	for _, d := range []struct {
		field string
		value int
	}{
		{"retention_days", b.RetentionDays},
		{"nearline_days", b.NearlineDays},
		{"noncurrent_days", b.NoncurrentDays},
		{"local_keep", b.LocalKeep},
	} {
		if d.value < 0 {
			return fmt.Errorf("forge backup %s %d must not be negative", d.field, d.value)
		}
	}
	if raw := strings.TrimSpace(b.StaleAfter); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("forge backup stale_after %q is not a valid duration: %w", b.StaleAfter, err)
		}
		if parsed <= 0 {
			return fmt.Errorf("forge backup stale_after %q must be positive", b.StaleAfter)
		}
	}
	if raw := strings.TrimSpace(b.NtfyURL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return fmt.Errorf("forge backup ntfy_url %q is not a valid URL", b.NtfyURL)
		}
	}
	return nil
}

// ForgeSyncdServiceConfig is [forge.services.syncd].
type ForgeSyncdServiceConfig struct {
	// Enabled colocates grove-syncd on the forge VM. Nil means enabled — the
	// colocation IS the design (one box, one TLS story).
	Enabled *bool `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Colocate grove-syncd on the forge VM,default=true"`
	// Port is the TLS bind port. grove-syncd refuses a non-loopback bind
	// without TLS, and this module never disables that.
	Port int `yaml:"port,omitempty" toml:"port,omitempty" jsonschema:"description=grove-syncd TLS bind port,default=8788"`
}

// SyncdEnabled reports whether grove-syncd is colocated (nil-safe, default on).
func (s *ForgeServicesConfig) SyncdEnabled() bool {
	if s == nil || s.Syncd == nil || s.Syncd.Enabled == nil {
		return true
	}
	return *s.Syncd.Enabled
}

// EffectiveSyncdPort resolves the syncd bind port.
func (s *ForgeServicesConfig) EffectiveSyncdPort() int {
	if s == nil || s.Syncd == nil || s.Syncd.Port <= 0 {
		return DefaultForgeSyncdPort
	}
	return s.Syncd.Port
}

// EffectiveTLSMode resolves TLSMode. An ACME mode with no domain degrades to
// self-signed rather than rendering a plan that cannot possibly succeed;
// Validate reports the misconfiguration to whoever asked.
func (s *ForgeServicesConfig) EffectiveTLSMode() string {
	raw := ""
	if s != nil {
		raw = strings.ToLower(strings.TrimSpace(s.TLSMode))
	}
	if raw == ForgeTLSACME && s.EffectiveDomain() != "" {
		return ForgeTLSACME
	}
	return ForgeTLSSelfSigned
}

// EffectiveDomain resolves Domain (nil-safe, trimmed, lowercased).
func (s *ForgeServicesConfig) EffectiveDomain() string {
	if s == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s.Domain))
}

func forgeStringDefault(raw, def string) string {
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		return trimmed
	}
	return def
}

// IsConfigured reports whether a usable forge is declared. An empty or
// missing block means "no forge", which is the normal state for every
// worktree today.
func (f *ForgeConfig) IsConfigured() bool {
	return f != nil && strings.TrimSpace(f.URL) != ""
}

// Validate checks structural validity. Like SyncConfig.Validate it is NOT
// called on the config load path: a key nothing consumes yet must never start
// failing config loads. Consumers that activate the forge call it explicitly.
func (f *ForgeConfig) Validate() error {
	if f == nil {
		return nil
	}
	if raw := strings.TrimSpace(f.URL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("forge url %q is not a valid URL: %w", f.URL, err)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
		default:
			return fmt.Errorf("forge url %q must use http or https, got scheme %q", f.URL, u.Scheme)
		}
		if u.Host == "" {
			return fmt.Errorf("forge url %q has no host", f.URL)
		}
	}
	if name := strings.TrimSpace(f.RemoteName); name != "" {
		if err := validateRemoteName(name); err != nil {
			return err
		}
	}
	switch strings.ToLower(strings.TrimSpace(f.Provider)) {
	case "", ForgeProviderAuto, ForgeProviderForgejo, ForgeProviderGitHub:
	default:
		return fmt.Errorf("forge provider %q must be one of %q, %q, %q", f.Provider,
			ForgeProviderAuto, ForgeProviderForgejo, ForgeProviderGitHub)
	}
	if strings.EqualFold(strings.TrimSpace(f.Provider), ForgeProviderForgejo) && !f.IsConfigured() {
		return fmt.Errorf("forge provider %q needs a [forge] url to poll", ForgeProviderForgejo)
	}
	if err := f.Infra.Validate(); err != nil {
		return err
	}
	if err := f.Services.Validate(); err != nil {
		return err
	}
	if err := f.Backup.Validate(); err != nil {
		return err
	}
	if err := f.Wireguard.Validate(); err != nil {
		return err
	}
	if f.Poll != nil {
		// Report a typo'd duration to whoever asked for validation. The runtime
		// path never consults this: EffectiveInterval/EffectiveStaleAfter fall
		// back to the defaults, because an unparseable poll cadence must not be
		// a reason the daemon fails to boot.
		for _, d := range []struct{ field, raw string }{
			{"interval", f.Poll.Interval},
			{"stale_after", f.Poll.StaleAfter},
		} {
			raw := strings.TrimSpace(d.raw)
			if raw == "" {
				continue
			}
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("forge poll %s %q is not a valid duration: %w", d.field, d.raw, err)
			}
			if parsed <= 0 {
				return fmt.Errorf("forge poll %s %q must be positive", d.field, d.raw)
			}
		}
	}
	return nil
}

// Validate checks the [forge.infra] block's structure. Like ForgeConfig.Validate
// it is never called on the config load path.
func (i *ForgeInfraConfig) Validate() error {
	if i == nil {
		return nil
	}
	if cidr := strings.TrimSpace(i.CIDR); cidr != "" {
		if err := validateForgeCIDR("forge infra cidr", cidr); err != nil {
			return err
		}
	}
	if name := strings.TrimSpace(i.VMName); name != "" {
		// The name becomes a GCE resource name and a firewall-rule prefix, both
		// of which are RFC1035-shaped. Rejecting here beats a terraform apply
		// failing halfway through creating a firewall rule.
		if err := validateForgeResourceName("forge infra vm_name", name); err != nil {
			return err
		}
	}
	if i.DiskSizeGB < 0 {
		return fmt.Errorf("forge infra disk_size_gb %d must be positive", i.DiskSizeGB)
	}
	return nil
}

// Validate checks the [forge.services] block's structure.
func (s *ForgeServicesConfig) Validate() error {
	if s == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(s.TLSMode)) {
	case "", ForgeTLSSelfSigned:
	case ForgeTLSACME:
		if s.EffectiveDomain() == "" {
			return fmt.Errorf("forge services tls_mode %q needs a domain", ForgeTLSACME)
		}
		if strings.TrimSpace(s.ACMEDNSProvider) == "" {
			// HTTP-01 would need :80 open to the world, which the module
			// refuses; so an ACME setup without a DNS provider has no challenge
			// it can actually complete.
			return fmt.Errorf("forge services tls_mode %q needs acme_dns_provider (only the DNS-01 challenge is offered — HTTP-01 would require 0.0.0.0/0 on :80)", ForgeTLSACME)
		}
		if strings.TrimSpace(s.ACMEEmail) == "" {
			return fmt.Errorf("forge services tls_mode %q needs acme_email", ForgeTLSACME)
		}
	default:
		return fmt.Errorf("forge services tls_mode %q must be %q or %q", s.TLSMode, ForgeTLSSelfSigned, ForgeTLSACME)
	}
	if s.Forgejo != nil {
		if err := validateForgePort("forge services forgejo http_port", s.Forgejo.HTTPPort); err != nil {
			return err
		}
		if v := strings.TrimSpace(s.Forgejo.Version); v != "" {
			if strings.TrimSpace(s.Forgejo.SHA256) == "" {
				return fmt.Errorf("forge services forgejo version %q needs a sha256 (a pinned version with an unverified download is not a pin)", v)
			}
		}
		if sum := strings.TrimSpace(s.Forgejo.SHA256); sum != "" {
			if err := validateForgeSHA256("forge services forgejo sha256", sum); err != nil {
				return err
			}
		}
	}
	if s.Syncd != nil {
		if err := validateForgePort("forge services syncd port", s.Syncd.Port); err != nil {
			return err
		}
	}
	if s.Forgejo != nil && s.Syncd != nil &&
		s.Forgejo.EffectiveHTTPPort() == s.EffectiveSyncdPort() {
		return fmt.Errorf("forge services forgejo http_port and syncd port are both %d — colocated services cannot share a port", s.EffectiveSyncdPort())
	}
	return nil
}

// ValidateForProvision is the STRICTER gate `grove forge up`/`plan` runs: it
// additionally requires the inputs terraform has no default for, so a
// provision aborts while it is still free rather than halfway through an apply.
// Validate stays the structural check for everyone else.
func (f *ForgeConfig) ValidateForProvision() error {
	if err := f.Validate(); err != nil {
		return err
	}
	if f == nil || f.Infra == nil {
		return fmt.Errorf("no [forge.infra] block: `grove forge up` needs project, ssh_user and cidr")
	}
	var missing []string
	if strings.TrimSpace(f.Infra.Project) == "" {
		missing = append(missing, "project")
	}
	if strings.TrimSpace(f.Infra.SSHUser) == "" {
		missing = append(missing, "ssh_user")
	}
	if strings.TrimSpace(f.Infra.CIDR) == "" {
		missing = append(missing, "cidr")
	}
	if len(missing) > 0 {
		return fmt.Errorf("[forge.infra] is missing %s (no terraform defaults exist for these)", strings.Join(missing, ", "))
	}
	if f.Services == nil || f.Services.Forgejo == nil ||
		strings.TrimSpace(f.Services.Forgejo.Version) == "" ||
		strings.TrimSpace(f.Services.Forgejo.SHA256) == "" {
		return fmt.Errorf("[forge.services.forgejo] needs both version and sha256 — the forge binary is installed by pinned download, never by \"latest\"")
	}
	return nil
}

// validateForgeCIDR rejects a malformed CIDR and, emphatically, the open one.
// The terraform module refuses 0.0.0.0/0 too; this is the copy of that rule
// that fires before terraform is ever invoked.
func validateForgeCIDR(field, cidr string) error {
	if cidr == "0.0.0.0/0" || cidr == "::/0" {
		return fmt.Errorf("%s %q is the whole internet — restrict it to your operator address (e.g. 203.0.113.7/32)", field, cidr)
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("%s %q is not a valid CIDR: %w", field, cidr, err)
	}
	return nil
}

// validateForgeResourceName enforces the RFC1035 shape GCE requires of
// instance and firewall names.
func validateForgeResourceName(field, name string) error {
	if len(name) > 50 {
		// 50, not 63: the module appends "-allow-forgejo" and friends.
		return fmt.Errorf("%s %q is too long (max 50 chars — the module appends firewall-rule suffixes)", field, name)
	}
	if name[0] < 'a' || name[0] > 'z' {
		return fmt.Errorf("%s %q must start with a lowercase letter", field, name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return fmt.Errorf("%s %q contains illegal character %q (lowercase letters, digits and '-' only)", field, name, string(r))
		}
	}
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("%s %q may not end with '-'", field, name)
	}
	return nil
}

func validateForgePort(field string, port int) error {
	if port == 0 {
		return nil // absent — the effective accessor supplies the default
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s %d is not a valid TCP port", field, port)
	}
	return nil
}

func validateForgeSHA256(field, sum string) error {
	if len(sum) != 64 {
		return fmt.Errorf("%s must be 64 hex characters, got %d", field, len(sum))
	}
	for _, r := range sum {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return fmt.Errorf("%s contains non-hex character %q", field, string(r))
		}
	}
	return nil
}

// validateRemoteName rejects names git itself would reject, and names that
// would be read as flags by the git commands that eventually carry them.
func validateRemoteName(name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("forge remote_name %q may not start with '-'", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return fmt.Errorf("forge remote_name %q contains illegal character %q", name, string(r))
		}
	}
	return nil
}

// Forge decodes the [forge] block from a loaded Config.
//
// It returns (nil, nil) when no [forge] key is present — the overwhelmingly
// common case — so callers gate on presence rather than on a zero struct.
func (c *Config) Forge() (*ForgeConfig, error) {
	if c == nil {
		return nil, nil
	}
	if _, ok := c.Extensions[ForgeExtensionKey]; !ok {
		return nil, nil
	}
	var cfg ForgeConfig
	if err := c.UnmarshalExtension(ForgeExtensionKey, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode [forge] config: %w", err)
	}
	return &cfg, nil
}
