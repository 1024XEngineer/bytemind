package extensions

import (
	"sync"
	"testing"
	"time"

	skillspkg "github.com/1024XEngineer/bytemind/internal/skills"
)

func TestSkillAdapterSyncCachesSkillExtensions(t *testing.T) {
	adapter := newSkillAdapter()
	now := time.Now().UTC()
	items := adapter.Sync(skillspkg.Catalog{Skills: []skillspkg.Skill{{
		Name:         "review",
		Title:        "review",
		Description:  "desc",
		Scope:        skillspkg.ScopeProject,
		SourceDir:    `C:\\repo\\.bytemind\\skills\\review`,
		Instruction:  "body",
		DiscoveredAt: now,
	}}})
	if len(items) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(items))
	}
	if items[0].ID != "skill.review" {
		t.Fatalf("unexpected id: %q", items[0].ID)
	}
	cached := adapter.Sync(skillspkg.Catalog{Skills: []skillspkg.Skill{{
		Name:         "review",
		Title:        "review",
		Description:  "desc",
		Scope:        skillspkg.ScopeProject,
		SourceDir:    `C:\\repo\\.bytemind\\skills\\review`,
		Instruction:  "body",
		DiscoveredAt: now,
	}}})
	if len(cached) != 1 {
		t.Fatalf("expected cached extension, got %d", len(cached))
	}
}

func TestSkillAdapterFromSkillMarksManifestOnlyAsDegraded(t *testing.T) {
	adapter := newSkillAdapter()
	item := adapter.FromSkill(skillspkg.Skill{
		Name:         "review",
		Title:        "review",
		Scope:        skillspkg.ScopeProject,
		SourceDir:    `C:\\repo\\.bytemind\\skills\\review`,
		DiscoveredAt: time.Now().UTC(),
	})
	if item.Status != ExtensionStatusDegraded {
		t.Fatalf("expected degraded status, got %q", item.Status)
	}
	if item.Health.Status != ExtensionStatusDegraded {
		t.Fatalf("expected degraded health, got %q", item.Health.Status)
	}
}

func TestSkillAdapterFromSkillKeepsPrefixedNameIDCompatibility(t *testing.T) {
	adapter := newSkillAdapter()
	item := adapter.FromSkill(skillspkg.Skill{
		Name:         "skill.review",
		Title:        "skill.review",
		Scope:        skillspkg.ScopeProject,
		SourceDir:    `C:\\repo\\.bytemind\\skills\\skill.review`,
		DiscoveredAt: time.Now().UTC(),
	})
	if item.ID != "skill.skill.review" {
		t.Fatalf("expected compat id skill.skill.review, got %q", item.ID)
	}
}

func TestSkillAdapterSyncConcurrentDoesNotPanic(t *testing.T) {
	adapter := newSkillAdapter()
	base := time.Now().UTC()
	catalogs := []skillspkg.Catalog{
		{Skills: []skillspkg.Skill{{
			Name:         "review",
			Title:        "review",
			Description:  "desc",
			Scope:        skillspkg.ScopeProject,
			SourceDir:    `C:\\repo\\.bytemind\\skills\\review`,
			Instruction:  "body",
			DiscoveredAt: base,
		}}},
		{Skills: []skillspkg.Skill{{
			Name:         "plan",
			Title:        "plan",
			Description:  "desc",
			Scope:        skillspkg.ScopeProject,
			SourceDir:    `C:\\repo\\.bytemind\\skills\\plan`,
			Instruction:  "body",
			DiscoveredAt: base.Add(time.Second),
		}}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				items := adapter.Sync(catalogs[(idx+j)%len(catalogs)])
				if len(items) != 1 {
					t.Errorf("expected 1 extension, got %d", len(items))
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
