package mention

import (
	"bufio"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const defaultSearchLimit = 15
const (
	mentionTrieRecallMultiplier = 8
	mentionTrieMinRecall        = 64
	mentionIgnoreMaxLineBytes   = 4 * 1024 * 1024
)

var (
	mentionIndexRefreshInterval    = 10 * time.Second
	mentionIndexDefaultMaxFiles    = 6000
	mentionIndexDefaultMaxVisits   = 25000
	mentionIndexDefaultMaxDirs     = 5000
	mentionIndexDefaultMaxDuration = 750 * time.Millisecond
)

type Token struct {
	Query string
	Start int
	End   int
}

type Candidate struct {
	Path     string
	BaseName string
	TypeTag  string
}

type IndexStats struct {
	Count     int
	MaxFiles  int
	Truncated bool
	Ready     bool
}

type mentionIgnoreMatcher struct {
	exact map[string]struct{}
	globs []string
}

type mentionTrieNode struct {
	children map[rune]*mentionTrieNode
	indices  []int
}

type mentionTrie struct {
	root *mentionTrieNode
}

type WorkspaceFileIndex struct {
	mu        sync.RWMutex
	root      string
	ready     bool
	building  bool
	lastBuild time.Time
	files     []Candidate
	trie      *mentionTrie
	truncated bool
	maxFiles  int
}

func NewWorkspaceFileIndex(workspace string) *WorkspaceFileIndex {
	return &WorkspaceFileIndex{
		root:     strings.TrimSpace(workspace),
		maxFiles: mentionMaxFilesFromEnv(),
	}
}

func NewStaticWorkspaceFileIndex(candidates []Candidate, maxFiles int, truncated bool) *WorkspaceFileIndex {
	defaultMax := maxFiles <= 0
	copied := make([]Candidate, 0, len(candidates))
	for _, item := range candidates {
		candidate := item
		candidate.Path = filepath.ToSlash(strings.TrimSpace(candidate.Path))
		if candidate.Path == "" {
			continue
		}
		if strings.TrimSpace(candidate.BaseName) == "" {
			candidate.BaseName = filepath.Base(candidate.Path)
		}
		if strings.TrimSpace(candidate.TypeTag) == "" {
			candidate.TypeTag = mentionTypeTag(candidate.Path)
		}
		copied = append(copied, candidate)
	}
	if defaultMax {
		maxFiles = len(copied)
	}
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].Path < copied[j].Path
	})
	return &WorkspaceFileIndex{
		ready:     true,
		files:     copied,
		trie:      newMentionTrie(copied),
		truncated: truncated,
		maxFiles:  maxFiles,
	}
}

func (idx *WorkspaceFileIndex) Search(query string, limit int) []Candidate {
	return idx.SearchWithRecency(query, limit, nil)
}

func (idx *WorkspaceFileIndex) SearchWithRecency(query string, limit int, recency map[string]int) []Candidate {
	if idx == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	idx.ensureInitialBuild()
	idx.ensureFreshAsync()
	files, trie := idx.snapshotSearchData()
	if len(files) == 0 {
		return nil
	}

	q := strings.ToLower(filepath.ToSlash(strings.TrimSpace(query)))
	candidateIndices, prefixSet := mentionCandidateIndices(files, trie, q, limit)
	type rankedMention struct {
		candidate Candidate
		score     int
		recency   int
	}
	ranked := make([]rankedMention, 0, len(candidateIndices))
	for _, idx := range candidateIndices {
		file := files[idx]
		_, prefixHit := prefixSet[idx]
		score, ok := scoreCandidate(file, q, prefixHit)
		if !ok {
			continue
		}
		recent := 0
		if recency != nil {
			recent = recency[file.Path]
		}
		ranked = append(ranked, rankedMention{
			candidate: file,
			score:     score,
			recency:   recent,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].recency == ranked[j].recency {
				return ranked[i].candidate.Path < ranked[j].candidate.Path
			}
			return ranked[i].recency > ranked[j].recency
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	out := make([]Candidate, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.candidate)
	}
	return out
}

func (idx *WorkspaceFileIndex) Prewarm() {
	if idx == nil {
		return
	}
	idx.ensureInitialBuild()
}

func (idx *WorkspaceFileIndex) Stats() IndexStats {
	if idx == nil {
		return IndexStats{}
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return IndexStats{
		Count:     len(idx.files),
		MaxFiles:  idx.maxFiles,
		Truncated: idx.truncated,
		Ready:     idx.ready,
	}
}

func (idx *WorkspaceFileIndex) snapshotSearchData() ([]Candidate, *mentionTrie) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if len(idx.files) == 0 {
		return nil, nil
	}
	out := make([]Candidate, len(idx.files))
	copy(out, idx.files)
	return out, idx.trie
}

func (idx *WorkspaceFileIndex) ensureInitialBuild() {
	if idx == nil {
		return
	}
	idx.mu.RLock()
	ready := idx.ready
	idx.mu.RUnlock()
	if ready {
		return
	}
	idx.rebuildBlocking()
}

func (idx *WorkspaceFileIndex) ensureFreshAsync() {
	if idx == nil {
		return
	}
	idx.mu.RLock()
	stale := idx.shouldRebuildLocked()
	ready := idx.ready
	building := idx.building
	idx.mu.RUnlock()

	if !ready {
		idx.rebuildBlocking()
		return
	}
	if stale && !building {
		go idx.rebuildBlocking()
	}
}

func (idx *WorkspaceFileIndex) rebuildBlocking() {
	if idx == nil {
		return
	}
	idx.mu.Lock()
	if idx.building || !idx.shouldRebuildLocked() {
		idx.mu.Unlock()
		return
	}
	idx.building = true
	root := idx.root
	maxFiles := idx.maxFiles
	idx.mu.Unlock()

	matcher := loadMentionIgnoreMatcher(root)
	files, truncated := buildMentionIndex(root, maxFiles, matcher)

	idx.mu.Lock()
	idx.files = files
	idx.trie = newMentionTrie(files)
	idx.truncated = truncated
	idx.ready = true
	idx.lastBuild = time.Now()
	idx.building = false
	idx.mu.Unlock()
}

func (idx *WorkspaceFileIndex) shouldRebuildLocked() bool {
	if idx == nil {
		return false
	}
	if strings.TrimSpace(idx.root) == "" {
		return !idx.ready
	}
	if !idx.ready || idx.lastBuild.IsZero() {
		return true
	}
	return time.Since(idx.lastBuild) >= mentionIndexRefreshInterval
}

func buildMentionIndex(root string, maxFiles int, matcher mentionIgnoreMatcher) ([]Candidate, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, false
	}
	if maxFiles <= 0 {
		maxFiles = mentionIndexDefaultMaxFiles
	}
	maxVisits := mentionMaxVisitsFromEnv()
	maxDirs := mentionMaxDirsFromEnv()
	maxDuration := mentionMaxDurationFromEnv()
	started := time.Now()

	files := make([]Candidate, 0, minInt(maxFiles, 512))
	truncated := false
	visits := 0
	dirs := 0
	_ = filepath.WalkDir(root, func(pathAbs string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if pathAbs == root {
			return nil
		}
		visits++
		if visits > maxVisits {
			truncated = true
			return fs.SkipAll
		}
		if maxDuration > 0 && time.Since(started) > maxDuration {
			truncated = true
			return fs.SkipAll
		}

		rel, relErr := filepath.Rel(root, pathAbs)
		if relErr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()

		if d.IsDir() {
			dirs++
			if dirs > maxDirs {
				truncated = true
				return fs.SkipAll
			}
			if shouldSkipMentionDir(name) || matcher.SkipDir(name, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if shouldSkipMentionFile(name) || matcher.SkipFile(name, rel) {
			return nil
		}
		if rel == "" || rel == "." {
			return nil
		}

		files = append(files, Candidate{
			Path:     rel,
			BaseName: filepath.Base(rel),
			TypeTag:  mentionTypeTag(rel),
		})
		if len(files) >= maxFiles {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, truncated
}

func mentionCandidateIndices(files []Candidate, trie *mentionTrie, query string, limit int) ([]int, map[int]struct{}) {
	if len(files) == 0 {
		return nil, nil
	}
	if query == "" {
		all := make([]int, 0, len(files))
		for i := range files {
			all = append(all, i)
		}
		return all, nil
	}

	// Trie provides fast basename prefix recall; fuzzy matching is then
	// applied on recalled candidates, with full fallback when recall is small.
	recall := maxInt(mentionTrieMinRecall, limit*mentionTrieRecallMultiplier)
	prefixHits := []int{}
	if trie != nil {
		prefixHits = trie.prefixIndices(query, recall)
	}
	prefixSet := make(map[int]struct{}, len(prefixHits))
	candidates := make([]int, 0, len(prefixHits))
	for _, idx := range prefixHits {
		if idx < 0 || idx >= len(files) {
			continue
		}
		if _, exists := prefixSet[idx]; exists {
			continue
		}
		prefixSet[idx] = struct{}{}
		candidates = append(candidates, idx)
	}

	// For short or low-recall queries, widen to full set so fuzzy subsequence
	// matching can still recover useful hits.
	if len(candidates) < limit*2 || len([]rune(query)) <= 1 {
		for i := range files {
			if _, exists := prefixSet[i]; exists {
				continue
			}
			candidates = append(candidates, i)
		}
	}
	if len(prefixSet) == 0 {
		return candidates, nil
	}
	return candidates, prefixSet
}

func scoreCandidate(file Candidate, query string, prefixHit bool) (int, bool) {
	path := strings.ToLower(filepath.ToSlash(strings.TrimSpace(file.Path)))
	base := strings.ToLower(strings.TrimSpace(file.BaseName))
	if base == "" {
		base = strings.ToLower(filepath.Base(path))
	}
	if path == "" {
		return 0, false
	}

	if query == "" {
		score := 1000 - len([]rune(path))
		if prefixHit {
			score += 60
		}
		return score, true
	}

	score := 0
	matched := false

	if base == query {
		score += 6000
		matched = true
	} else if strings.HasPrefix(base, query) {
		score += 4200
		matched = true
	}
	if strings.HasPrefix(path, query) {
		score += 1200
		matched = true
	}

	if fuzzyBase, ok := mentionFuzzyScore(query, base); ok {
		score += 3000 + fuzzyBase
		matched = true
	}
	if fuzzyPath, ok := mentionFuzzyScore(query, path); ok {
		score += 1200 + fuzzyPath
		matched = true
	}

	if !matched {
		return 0, false
	}
	if prefixHit {
		score += 480
	}
	score -= len([]rune(path)) / 3
	return score, true
}

func mentionFuzzyScore(query, target string) (int, bool) {
	queryRunes := []rune(strings.TrimSpace(query))
	targetRunes := []rune(strings.TrimSpace(target))
	if len(queryRunes) == 0 {
		return 0, true
	}
	if len(targetRunes) == 0 {
		return 0, false
	}

	positions, ok := mentionFuzzyPositions(queryRunes, targetRunes)
	if !ok {
		return 0, false
	}

	score := 0
	baseStart := basenameStartIndex(targetRunes)
	prev := -1
	for i, pos := range positions {
		score += 12
		if i == 0 && pos == 0 {
			score += 30
		}
		if mentionBoundary(targetRunes, pos) {
			score += 14
		}
		if pos >= baseStart {
			score += 8
		}
		if prev >= 0 {
			gap := pos - prev - 1
			if gap == 0 {
				score += 18
			} else {
				score -= minInt(gap*2, 30)
			}
		}
		prev = pos
	}
	score -= len(targetRunes) / 2
	return score, true
}

func mentionFuzzyPositions(query, target []rune) ([]int, bool) {
	if len(query) == 0 {
		return nil, true
	}
	positions := make([]int, 0, len(query))
	searchFrom := 0
	for _, want := range query {
		found := -1
		for i := searchFrom; i < len(target); i++ {
			if target[i] == want {
				found = i
				searchFrom = i + 1
				break
			}
		}
		if found == -1 {
			return nil, false
		}
		positions = append(positions, found)
	}
	return positions, true
}

func mentionBoundary(target []rune, pos int) bool {
	if pos <= 0 {
		return true
	}
	switch target[pos-1] {
	case '/', '\\', '_', '-', '.', ' ', '[', '(', '{':
		return true
	default:
		return false
	}
}

func basenameStartIndex(path []rune) int {
	if len(path) == 0 {
		return 0
	}
	last := 0
	for i, r := range path {
		if r == '/' || r == '\\' {
			last = i + 1
		}
	}
	return last
}

func newMentionTrie(files []Candidate) *mentionTrie {
	trie := &mentionTrie{
		root: &mentionTrieNode{children: map[rune]*mentionTrieNode{}},
	}
	for i, candidate := range files {
		key := strings.ToLower(strings.TrimSpace(candidate.BaseName))
		if key == "" {
			key = strings.ToLower(filepath.Base(candidate.Path))
		}
		if key == "" {
			continue
		}
		trie.insert(key, i)
	}
	return trie
}

func (t *mentionTrie) insert(key string, idx int) {
	if t == nil || t.root == nil || key == "" {
		return
	}
	node := t.root
	for _, r := range []rune(key) {
		if node.children == nil {
			node.children = map[rune]*mentionTrieNode{}
		}
		child := node.children[r]
		if child == nil {
			child = &mentionTrieNode{children: map[rune]*mentionTrieNode{}}
			node.children[r] = child
		}
		node = child
		node.indices = append(node.indices, idx)
	}
}

func (t *mentionTrie) prefixIndices(prefix string, limit int) []int {
	if t == nil || t.root == nil || strings.TrimSpace(prefix) == "" {
		return nil
	}
	if limit <= 0 {
		limit = mentionTrieMinRecall
	}
	node := t.root
	for _, r := range []rune(strings.ToLower(strings.TrimSpace(prefix))) {
		next := node.children[r]
		if next == nil {
			return nil
		}
		node = next
	}
	if len(node.indices) <= limit {
		out := make([]int, len(node.indices))
		copy(out, node.indices)
		return out
	}
	out := make([]int, limit)
	copy(out, node.indices[:limit])
	return out
}

func shouldSkipMentionDir(name string) bool {
	if strings.TrimSpace(name) == "" {
		return true
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, ".") {
		return true
	}
	switch lower {
	case "node_modules", "vendor", "dist", "build":
		return true
	default:
		return false
	}
}

func shouldSkipMentionFile(name string) bool {
	if strings.TrimSpace(name) == "" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func loadMentionIgnoreMatcher(workspace string) mentionIgnoreMatcher {
	matcher := mentionIgnoreMatcher{
		exact: make(map[string]struct{}, 16),
		globs: make([]string, 0, 8),
	}

	for _, item := range []string{"node_modules", "vendor", "dist", "build"} {
		matcher.addRule(item)
	}
	if env := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_IGNORE")); env != "" {
		for _, part := range strings.Split(env, ",") {
			matcher.addRule(part)
		}
	}

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return matcher
	}
	ignorePath := filepath.Join(workspace, ".bytemindignore")
	f, err := os.Open(ignorePath)
	if err != nil {
		return matcher
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), mentionIgnoreMaxLineBytes)
	for scanner.Scan() {
		matcher.addRule(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("mention: failed to parse %s: %v", ignorePath, err)
	}
	return matcher
}

func (m *mentionIgnoreMatcher) addRule(raw string) {
	if m == nil {
		return
	}
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	line = strings.Trim(filepath.ToSlash(line), "/")
	line = strings.ToLower(line)
	if line == "" {
		return
	}
	if strings.ContainsAny(line, "*?[") || strings.Contains(line, "/") {
		m.globs = append(m.globs, line)
		return
	}
	m.exact[line] = struct{}{}
}

func (m mentionIgnoreMatcher) SkipDir(name, relPath string) bool {
	return m.skip(name, relPath)
}

func (m mentionIgnoreMatcher) SkipFile(name, relPath string) bool {
	return m.skip(name, relPath)
}

func (m mentionIgnoreMatcher) skip(name, relPath string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	rel := strings.ToLower(strings.Trim(filepath.ToSlash(strings.TrimSpace(relPath)), "/"))
	if name == "" || rel == "" {
		return false
	}
	if _, ok := m.exact[name]; ok {
		return true
	}
	if _, ok := m.exact[rel]; ok {
		return true
	}
	for _, pattern := range m.globs {
		if ok, _ := path.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func mentionMaxFilesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_MAX_FILES"))
	if raw == "" {
		return mentionIndexDefaultMaxFiles
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return mentionIndexDefaultMaxFiles
	}
	return n
}

func mentionMaxVisitsFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_MAX_VISITS"))
	if raw == "" {
		return mentionIndexDefaultMaxVisits
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return mentionIndexDefaultMaxVisits
	}
	return n
}

func mentionMaxDirsFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_MAX_DIRS"))
	if raw == "" {
		return mentionIndexDefaultMaxDirs
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return mentionIndexDefaultMaxDirs
	}
	return n
}

func mentionMaxDurationFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_MAX_DURATION_MS"))
	if raw == "" {
		return mentionIndexDefaultMaxDuration
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return mentionIndexDefaultMaxDuration
	}
	return time.Duration(n) * time.Millisecond
}

func FindActiveToken(input string) (Token, bool) {
	runes := []rune(input)
	if len(runes) == 0 || unicode.IsSpace(runes[len(runes)-1]) {
		return Token{}, false
	}

	start := len(runes) - 1
	for start >= 0 && !unicode.IsSpace(runes[start]) {
		start--
	}
	tokenStart := start + 1
	token := runes[tokenStart:]
	if len(token) == 0 || token[0] != '@' {
		return Token{}, false
	}
	return Token{
		Query: string(token[1:]),
		Start: tokenStart,
		End:   len(runes),
	}, true
}

func InsertIntoInput(input string, token Token, path string) string {
	runes := []rune(input)
	if token.Start < 0 || token.End < token.Start || token.End > len(runes) {
		return input
	}

	mention := []rune("@" + filepath.ToSlash(strings.TrimSpace(path)))
	out := make([]rune, 0, len(runes)+len(mention)+1)
	out = append(out, runes[:token.Start]...)
	out = append(out, mention...)

	if token.End < len(runes) {
		if !unicode.IsSpace(runes[token.End]) {
			out = append(out, ' ')
		}
		out = append(out, runes[token.End:]...)
		return string(out)
	}
	out = append(out, ' ')
	return string(out)
}

func mentionTypeTag(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "go", "md", "txt", "json", "yaml", "yml", "toml", "ini":
		return ext
	case "ts", "tsx", "js", "jsx", "mjs", "cjs":
		return ext
	case "py", "java", "rs", "c", "h", "cpp", "hpp":
		return ext
	case "sh", "bash", "zsh", "fish", "ps1":
		return ext
	case "":
		return "file"
	default:
		if len(ext) <= 4 {
			return ext
		}
		return "file"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
