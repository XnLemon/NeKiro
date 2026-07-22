package contracts

import (
	"fmt"

	semver "github.com/Masterminds/semver/v3"
)

type InstallationSemanticRuleID string

const (
	InstallationRuleCanonicalPermissions InstallationSemanticRuleID = "INST-SEM-001"
	InstallationRuleMonotonicUpdate      InstallationSemanticRuleID = "INST-SEM-002"
	InstallationRuleTerminalTimestamp    InstallationSemanticRuleID = "INST-SEM-003"
	InstallationRuleImmutablePin         InstallationSemanticRuleID = "INST-SEM-004"
	InstallationRulePinnedVersionMatches InstallationSemanticRuleID = "INST-SEM-005"
)

type InstallationSemanticValidationError struct {
	RuleID InstallationSemanticRuleID
}

func (validationError *InstallationSemanticValidationError) Error() string {
	return fmt.Sprintf("installation semantic validation failed (%s)", validationError.RuleID)
}

func validateInstallationV2Semantics(installation Installation) error {
	for index := 1; index < len(installation.AcceptedPermissions); index++ {
		if installation.AcceptedPermissions[index-1] >= installation.AcceptedPermissions[index] {
			return &InstallationSemanticValidationError{RuleID: InstallationRuleCanonicalPermissions}
		}
	}
	if installation.InstalledAt.After(installation.UpdatedAt) {
		return &InstallationSemanticValidationError{RuleID: InstallationRuleMonotonicUpdate}
	}
	if installation.Status == "uninstalled" {
		if installation.UninstalledAt == nil || !installation.UninstalledAt.Equal(installation.UpdatedAt) {
			return &InstallationSemanticValidationError{RuleID: InstallationRuleTerminalTimestamp}
		}
	}
	constraint, err := semver.NewConstraint(installation.VersionConstraint)
	if err != nil {
		return &InstallationSemanticValidationError{RuleID: InstallationRulePinnedVersionMatches}
	}
	version, err := semver.StrictNewVersion(installation.InstalledVersion)
	if err != nil || !constraint.Check(version) {
		return &InstallationSemanticValidationError{RuleID: InstallationRulePinnedVersionMatches}
	}
	return nil
}

func ValidateInstallationImmutablePin(before, after Installation) error {
	if before.VersionConstraint != after.VersionConstraint ||
		before.InstalledVersion != after.InstalledVersion ||
		before.InstalledReleaseID != after.InstalledReleaseID ||
		len(before.AcceptedPermissions) != len(after.AcceptedPermissions) {
		return &InstallationSemanticValidationError{RuleID: InstallationRuleImmutablePin}
	}
	for index := range before.AcceptedPermissions {
		if before.AcceptedPermissions[index] != after.AcceptedPermissions[index] {
			return &InstallationSemanticValidationError{RuleID: InstallationRuleImmutablePin}
		}
	}
	return nil
}
