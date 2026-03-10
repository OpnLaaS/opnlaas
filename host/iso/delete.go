package iso

import (
	"errors"
	"fmt"
	"hash/crc32"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

var ErrStoredISONotFound = errors.New("stored iso not found")

type PurgeReport struct {
	ISOName              string   `json:"iso_name"`
	RemovedPaths         []string `json:"removed_paths"`
	UpdatedPXEProfileIPs []string `json:"updated_pxe_profile_ips"`
}

func PurgeStoredISOByName(name string) (report *PurgeReport, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		err = fmt.Errorf("iso name is required")
		return
	}

	var record *db.StoredISOImage
	if record, err = resolveStoredISOByIdentifier(name); err != nil {
		return
	}
	if record == nil {
		err = fmt.Errorf("%w: %s", ErrStoredISONotFound, name)
		return
	}

	report = &PurgeReport{
		ISOName:      record.Name,
		RemovedPaths: make([]string, 0, 8),
	}

	if err = purgeISOArtifacts(record, report); err != nil {
		return
	}

	if err = detachISOFromPXEProfiles(record.Name, report); err != nil {
		return
	}

	if err = db.StoredISOImages.Delete(record.Name); err != nil {
		return
	}

	return
}

func resolveStoredISOByIdentifier(identifier string) (record *db.StoredISOImage, err error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return
	}

	// Exact match first.
	if record, err = db.StoredISOImages.Select(identifier); err != nil || record != nil {
		return
	}

	// URL-decoded fallback (for route/path encodings from UI clients).
	if decoded, decErr := url.PathUnescape(identifier); decErr == nil {
		decoded = strings.TrimSpace(decoded)
		if decoded != "" && decoded != identifier {
			if record, err = db.StoredISOImages.Select(decoded); err != nil || record != nil {
				return
			}
		}
	}

	// Fallback search by canonical variants.
	var records []*db.StoredISOImage
	if records, err = db.StoredISOImages.SelectAll(); err != nil {
		return
	}

	lowerIdentifier := strings.ToLower(identifier)
	for _, rec := range records {
		if rec == nil {
			continue
		}

		name := strings.TrimSpace(rec.Name)
		if strings.EqualFold(name, identifier) {
			record = rec
			return
		}

		artifactAlias := isoArtifactDirName(name)
		if strings.EqualFold(artifactAlias, identifier) || strings.EqualFold(artifactAlias, lowerIdentifier) {
			record = rec
			return
		}

		if base := strings.TrimSpace(filepath.Base(rec.FullISOPath)); base != "" && strings.EqualFold(base, identifier) {
			record = rec
			return
		}
	}

	return
}

func purgeISOArtifacts(record *db.StoredISOImage, report *PurgeReport) (err error) {
	artifactRoots := uniqueNonEmptyStrings(
		config.Config.PXE.HTTPServer.Directory,
		config.Config.PXE.TFTPServer.Directory,
	)

	artifactDirName := isoArtifactDirName(record.Name)
	removedSet := make(map[string]struct{})
	pathsToRemove := make([]string, 0, 12)

	for _, root := range artifactRoots {
		pathsToRemove = append(pathsToRemove,
			filepath.Join(root, "artifacts", record.Name),
			filepath.Join(root, "artifacts", artifactDirName),
		)
	}

	pathsToRemove = append(pathsToRemove,
		strings.TrimSpace(record.FullISOPath),
		strings.TrimSpace(record.KernelPath),
		strings.TrimSpace(record.InitrdPath),
	)

	var storageDir string = strings.TrimSpace(config.Config.ISOs.StorageDir)
	if storageDir != "" {
		if base := filepath.Base(strings.TrimSpace(record.FullISOPath)); base != "" && base != "." && base != string(filepath.Separator) {
			pathsToRemove = append(pathsToRemove, filepath.Join(storageDir, base))
		}
	}

	// Remove directories first so nested files are cleaned in one pass.
	sort.Slice(pathsToRemove, func(i, j int) bool { return len(pathsToRemove[i]) > len(pathsToRemove[j]) })

	for _, target := range pathsToRemove {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, seen := removedSet[target]; seen {
			continue
		}

		var info os.FileInfo
		if info, err = os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				err = nil
				continue
			}
			err = fmt.Errorf("stat artifact path %s: %w", target, err)
			return
		}

		if info.IsDir() {
			if err = os.RemoveAll(target); err != nil {
				err = fmt.Errorf("remove artifact directory %s: %w", target, err)
				return
			}
		} else {
			if err = os.Remove(target); err != nil && !os.IsNotExist(err) {
				err = fmt.Errorf("remove artifact file %s: %w", target, err)
				return
			}
		}

		removedSet[target] = struct{}{}
		report.RemovedPaths = append(report.RemovedPaths, target)
		isoLog.Basicf("Purged ISO artifact path=%s iso=%s\n", target, record.Name)
	}

	return
}

func detachISOFromPXEProfiles(isoName string, report *PurgeReport) (err error) {
	var profiles []*db.HostPXEProfile
	if profiles, err = db.PXEProfilesAll(); err != nil {
		return
	}

	updated := make([]string, 0)
	for _, profile := range profiles {
		if profile == nil || strings.TrimSpace(profile.ISOName) != isoName {
			continue
		}

		profile.ISOName = ""
		if profile.TemplateData == nil {
			profile.TemplateData = map[string]string{}
		}
		profile.TemplateData["template.pxe.localboot"] = "true"

		if err = db.SavePXEProfile(profile); err != nil {
			err = fmt.Errorf("update host pxe profile for %s: %w", profile.ManagementIP, err)
			return
		}

		updated = append(updated, profile.ManagementIP)
	}

	sort.Strings(updated)
	report.UpdatedPXEProfileIPs = updated
	return
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isoArtifactDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "iso"
	}

	slug := toSlug(name)
	if slug == "" {
		slug = "iso"
	}

	sum := crc32.ChecksumIEEE([]byte(name))
	return fmt.Sprintf("%s-%08x", slug, sum)
}

func toSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
