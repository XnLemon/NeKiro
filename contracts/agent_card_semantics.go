package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type AgentCardSemanticRuleID string

const (
	AgentCardRuleUniqueSkillIDs             AgentCardSemanticRuleID = "AC-SEM-001"
	AgentCardRuleUniquePermissionIDs        AgentCardSemanticRuleID = "AC-SEM-002"
	AgentCardRuleRequiredPermissionDeclared AgentCardSemanticRuleID = "AC-SEM-003"
)

type AgentCardConformanceManifest struct {
	Cases []AgentCardConformanceCase `json:"cases"`
}

type AgentCardConformanceCase struct {
	ID            string                    `json:"id"`
	File          string                    `json:"file"`
	Valid         bool                      `json:"valid"`
	ViolatedRules []AgentCardSemanticRuleID `json:"violatedRules"`
	ContextFiles  []string                  `json:"contextFiles"`
}

type agentCardConformanceManifestJSON struct {
	Cases *[]agentCardConformanceCaseJSON `json:"cases"`
}

type agentCardConformanceCaseJSON struct {
	ID            *string                    `json:"id"`
	File          *string                    `json:"file"`
	Valid         *bool                      `json:"valid"`
	ViolatedRules *[]AgentCardSemanticRuleID `json:"violatedRules"`
	ContextFiles  *[]string                  `json:"contextFiles"`
}

type AgentCardSemanticViolation struct {
	RuleID AgentCardSemanticRuleID
	Path   string
}

func DecodeAgentCardConformanceManifest(data []byte) (AgentCardConformanceManifest, error) {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return AgentCardConformanceManifest{}, fmt.Errorf("decode Agent Card conformance manifest: %w", err)
	}

	var document agentCardConformanceManifestJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return AgentCardConformanceManifest{}, fmt.Errorf("decode Agent Card conformance manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return AgentCardConformanceManifest{}, fmt.Errorf("decode Agent Card conformance manifest: %w", err)
	}
	if document.Cases == nil {
		return AgentCardConformanceManifest{}, fmt.Errorf("agent card conformance manifest is missing cases")
	}
	if len(*document.Cases) == 0 {
		return AgentCardConformanceManifest{}, fmt.Errorf("agent card conformance manifest cases must not be empty")
	}

	manifest := AgentCardConformanceManifest{
		Cases: make([]AgentCardConformanceCase, 0, len(*document.Cases)),
	}
	caseIDs := make(map[string]struct{}, len(*document.Cases))
	for index, wireCase := range *document.Cases {
		manifestCase, err := decodeAgentCardConformanceCase(index, wireCase)
		if err != nil {
			return AgentCardConformanceManifest{}, err
		}
		if _, exists := caseIDs[manifestCase.ID]; exists {
			return AgentCardConformanceManifest{}, fmt.Errorf("agent card conformance manifest contains duplicate case id %q", manifestCase.ID)
		}
		caseIDs[manifestCase.ID] = struct{}{}
		manifest.Cases = append(manifest.Cases, manifestCase)
	}
	return manifest, nil
}

func decodeAgentCardConformanceCase(index int, wireCase agentCardConformanceCaseJSON) (AgentCardConformanceCase, error) {
	if wireCase.ID == nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %d is missing id", index)
	}
	if *wireCase.ID == "" {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %d id must not be empty", index)
	}
	if wireCase.File == nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q is missing file", *wireCase.ID)
	}
	if *wireCase.File == "" {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q file must not be empty", *wireCase.ID)
	}
	if err := validateAgentCardConformanceFixturePath(*wireCase.File); err != nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q file: %w", *wireCase.ID, err)
	}
	if wireCase.Valid == nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q is missing valid", *wireCase.ID)
	}
	if wireCase.ViolatedRules == nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q is missing violatedRules", *wireCase.ID)
	}
	if wireCase.ContextFiles == nil {
		return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q is missing contextFiles", *wireCase.ID)
	}
	if *wireCase.Valid && len(*wireCase.ViolatedRules) > 0 {
		return AgentCardConformanceCase{}, fmt.Errorf("valid Agent Card conformance case %q must not declare violated rules", *wireCase.ID)
	}

	ruleIDs := make(map[AgentCardSemanticRuleID]struct{}, len(*wireCase.ViolatedRules))
	for _, ruleID := range *wireCase.ViolatedRules {
		if !isAgentCardSemanticRuleID(ruleID) {
			return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q contains unknown semantic rule id %q", *wireCase.ID, ruleID)
		}
		if _, exists := ruleIDs[ruleID]; exists {
			return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q repeats semantic rule id %q", *wireCase.ID, ruleID)
		}
		ruleIDs[ruleID] = struct{}{}
	}

	contextFiles := make(map[string]struct{}, len(*wireCase.ContextFiles))
	for _, contextFile := range *wireCase.ContextFiles {
		if err := validateAgentCardConformanceFixturePath(contextFile); err != nil {
			return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q context file: %w", *wireCase.ID, err)
		}
		if contextFile == *wireCase.File {
			return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q uses its primary fixture as context", *wireCase.ID)
		}
		if _, exists := contextFiles[contextFile]; exists {
			return AgentCardConformanceCase{}, fmt.Errorf("agent card conformance case %q repeats context file %q", *wireCase.ID, contextFile)
		}
		contextFiles[contextFile] = struct{}{}
	}

	return AgentCardConformanceCase{
		ID:            *wireCase.ID,
		File:          *wireCase.File,
		Valid:         *wireCase.Valid,
		ViolatedRules: *wireCase.ViolatedRules,
		ContextFiles:  *wireCase.ContextFiles,
	}, nil
}

func rejectDuplicateJSONMemberNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}

	switch delimiter {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			memberToken, err := decoder.Token()
			if err != nil {
				return err
			}
			memberName, ok := memberToken.(string)
			if !ok {
				return fmt.Errorf("JSON object member name must be a string")
			}
			if _, exists := members[memberName]; exists {
				return fmt.Errorf("duplicate JSON object member %q", memberName)
			}
			members[memberName] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object has invalid closing delimiter")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array has invalid closing delimiter")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func validateAgentCardConformanceFixturePath(fixturePath string) error {
	if fixturePath == "" {
		return fmt.Errorf("fixture path must not be empty")
	}
	if strings.HasPrefix(fixturePath, "/") {
		return fmt.Errorf("fixture path must be relative")
	}
	if strings.Contains(fixturePath, "\\") {
		return fmt.Errorf("fixture path must use forward slashes")
	}
	if containsASCIIControl(fixturePath) {
		return fmt.Errorf("fixture path contains an ASCII control character")
	}
	if strings.ContainsAny(fixturePath, "%?#<>\"|*") {
		return fmt.Errorf("fixture path contains a noncanonical character")
	}
	if hasURIScheme(fixturePath) {
		return fmt.Errorf("fixture path must not contain a URI scheme")
	}
	if strings.ContainsRune(fixturePath, ':') {
		return fmt.Errorf("fixture path contains a nonportable colon")
	}

	for _, segment := range strings.Split(fixturePath, "/") {
		if segment == "" {
			return fmt.Errorf("fixture path contains an empty segment")
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("fixture path contains a traversal segment")
		}
		if strings.TrimRight(segment, " .") != segment {
			return fmt.Errorf("fixture path contains a platform-equivalent traversal segment")
		}
		if isWindowsReservedBasename(segment) {
			return fmt.Errorf("fixture path contains a Windows reserved device basename")
		}
	}
	return nil
}

func containsASCIIControl(value string) bool {
	for _, character := range value {
		if character <= 0x1f || character == 0x7f {
			return true
		}
	}
	return false
}

func isWindowsReservedBasename(segment string) bool {
	basename := segment
	if extension := strings.IndexByte(segment, '.'); extension >= 0 {
		basename = segment[:extension]
	}
	basename = strings.ToUpper(basename)
	if basename == "CON" || basename == "PRN" || basename == "AUX" || basename == "NUL" {
		return true
	}
	if len(basename) != 4 || basename[3] < '1' || basename[3] > '9' {
		return false
	}
	return basename[:3] == "COM" || basename[:3] == "LPT"
}

func hasURIScheme(value string) bool {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 {
		return false
	}
	if slash := strings.IndexByte(value, '/'); slash >= 0 && slash < colon {
		return false
	}
	for index := 0; index < colon; index++ {
		character := value[index]
		if index == 0 {
			if !isASCIIAlpha(character) {
				return false
			}
			continue
		}
		if !isASCIIAlpha(character) && (character < '0' || character > '9') && character != '+' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func isAgentCardSemanticRuleID(ruleID AgentCardSemanticRuleID) bool {
	switch ruleID {
	case AgentCardRuleUniqueSkillIDs,
		AgentCardRuleUniquePermissionIDs,
		AgentCardRuleRequiredPermissionDeclared:
		return true
	default:
		return false
	}
}

// EvaluateAgentCardSemantics evaluates rules that are outside JSON Schema's
// portable structural guarantees. Callers remain responsible for structural
// validation before treating an empty result as full Agent Card conformance.
func EvaluateAgentCardSemantics(card AgentCard) []AgentCardSemanticViolation {
	violations := make([]AgentCardSemanticViolation, 0)

	skillIDs := make(map[string]struct{}, len(card.Skills))
	for index, skill := range card.Skills {
		if _, exists := skillIDs[skill.ID]; exists {
			violations = append(violations, AgentCardSemanticViolation{
				RuleID: AgentCardRuleUniqueSkillIDs,
				Path:   fmt.Sprintf("/skills/%d/id", index),
			})
		}
		skillIDs[skill.ID] = struct{}{}
	}

	permissionIDs := make(map[string]struct{}, len(card.Permissions))
	for index, permission := range card.Permissions {
		if _, exists := permissionIDs[permission.ID]; exists {
			violations = append(violations, AgentCardSemanticViolation{
				RuleID: AgentCardRuleUniquePermissionIDs,
				Path:   fmt.Sprintf("/permissions/%d/id", index),
			})
		}
		permissionIDs[permission.ID] = struct{}{}
	}

	for skillIndex, skill := range card.Skills {
		for permissionIndex, permissionID := range skill.RequiredPermissions {
			if _, declared := permissionIDs[permissionID]; !declared {
				violations = append(violations, AgentCardSemanticViolation{
					RuleID: AgentCardRuleRequiredPermissionDeclared,
					Path: fmt.Sprintf(
						"/skills/%d/requiredPermissions/%d",
						skillIndex,
						permissionIndex,
					),
				})
			}
		}
	}

	return violations
}
