package skills

// SourceType identifies where a skill comes from.
type SourceType string

const (
	SourceTypeUser      SourceType = "user"
	SourceTypeEcosystem SourceType = "ecosystem"
	SourceTypeProject   SourceType = "project"
)

// SkillSource represents a discovered skill directory.
type SkillSource struct {
	Path string
	Type SourceType
}

// ResolvedSkill represents a skill that has been resolved from configuration
// to a physical path ready for syncing.
type ResolvedSkill struct {
	Name         string
	SourceType   SourceType
	PhysicalPath string
	Providers    []string
}
