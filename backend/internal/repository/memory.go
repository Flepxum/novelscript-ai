package repository

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

var ErrNotFound = errors.New("not found")

type MemoryRepository struct {
	mu       sync.RWMutex
	counters map[string]int

	projects map[string]domain.Project
	sources  map[string]domain.SourceDocument
	jobs     map[string]domain.GenerationJob
	versions map[string][]domain.ScriptVersion
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		counters: map[string]int{},
		projects: map[string]domain.Project{},
		sources:  map[string]domain.SourceDocument{},
		jobs:     map[string]domain.GenerationJob{},
		versions: map[string][]domain.ScriptVersion{},
	}
}

func (r *MemoryRepository) NextID(prefix string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[prefix]++
	return fmt.Sprintf("%s_%03d", prefix, r.counters[prefix])
}

func (r *MemoryRepository) SaveProject(project domain.Project) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects[project.ID] = project
}

func (r *MemoryRepository) ListProjects() []domain.Project {
	r.mu.RLock()
	defer r.mu.RUnlock()
	projects := make([]domain.Project, 0, len(r.projects))
	for _, project := range r.projects {
		projects = append(projects, project)
	}
	return projects
}

func (r *MemoryRepository) GetProject(id string) (domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	project, ok := r.projects[id]
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return project, nil
}

func (r *MemoryRepository) SaveSource(source domain.SourceDocument) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[source.ProjectID] = source
}

func (r *MemoryRepository) GetSource(projectID string) (domain.SourceDocument, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.sources[projectID]
	if !ok {
		return domain.SourceDocument{}, ErrNotFound
	}
	return source, nil
}

func (r *MemoryRepository) SaveJob(job domain.GenerationJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job.UpdatedAt = time.Now()
	r.jobs[job.ID] = job
}

func (r *MemoryRepository) GetJob(id string) (domain.GenerationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[id]
	if !ok {
		return domain.GenerationJob{}, ErrNotFound
	}
	return job, nil
}

func (r *MemoryRepository) SaveVersion(version domain.ScriptVersion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[version.ProjectID] = append(r.versions[version.ProjectID], version)
}

func (r *MemoryRepository) LatestVersion(projectID string) (domain.ScriptVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.versions[projectID]
	if len(versions) == 0 {
		return domain.ScriptVersion{}, ErrNotFound
	}
	return versions[len(versions)-1], nil
}

func (r *MemoryRepository) GetVersion(projectID, versionID string) (domain.ScriptVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, version := range r.versions[projectID] {
		if version.ID == versionID {
			return version, nil
		}
	}
	return domain.ScriptVersion{}, ErrNotFound
}

func (r *MemoryRepository) ListVersions(projectID string) []domain.ScriptVersion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.versions[projectID]
	result := make([]domain.ScriptVersion, len(versions))
	copy(result, versions)
	return result
}
