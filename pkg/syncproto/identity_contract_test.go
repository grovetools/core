package syncproto

import (
	"encoding/json"
	"testing"
)

func TestV3RegistrationGoldenAndExplicitIntent(t *testing.T) {
	req := RegisterRequest{
		RequestIdentity:     RequestIdentity{ProtocolVersion: 3, IdempotencyKey: "register-1", DeviceID: "device-1"},
		Intent:              RegistrationIntentCreateSibling,
		Subject:             "github.com/Me/Core",
		NotespaceName:       "core-personal",
		Kind:                "repo",
		ProposedNotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol_version":3,"idempotency_key":"register-1","device_id":"device-1","intent":"create-sibling","subject":"github.com/Me/Core","notespace_name":"core-personal","kind":"repo","proposed_notespace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV"}`
	if string(data) != want {
		t.Fatalf("register JSON\n got: %s\nwant: %s", data, want)
	}
	var roundTrip RegisterRequest
	if err := json.Unmarshal(data, &roundTrip); err != nil || roundTrip != req {
		t.Fatalf("register round trip = %+v, %v", roundTrip, err)
	}
	if validation := roundTrip.Validate(); validation != nil {
		t.Fatalf("valid register rejected: %v", validation)
	}
	missingIntent := roundTrip
	missingIntent.Intent = ""
	if validation := missingIntent.Validate(); validation == nil || validation.Code != ErrorRegistrationConflict {
		t.Fatalf("missing intent validation = %+v", validation)
	}
}

func TestV3DataWireHasIDAndDisplayOnlyName(t *testing.T) {
	req := PushRequest{ProtocolVersion: 3, NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", NotespaceName: "renamable", OriginID: "origin"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["notespace"] != req.NotespaceID.String() || raw["notespace_name"] != "renamable" {
		t.Fatalf("wrong v3 identity shape: %s", data)
	}
	if _, exists := raw["workspace"]; exists {
		t.Fatalf("legacy name-keyed field leaked into v3: %s", data)
	}
}

func TestLegacyDataRejectedButV2DeviceSignatureContractRetained(t *testing.T) {
	for _, version := range []int{ProtocolVersionLegacy, ProtocolVersionDeviceSession, 0} {
		err := ValidateDataRequest(version, "display-name")
		if err == nil || err.Code != ErrorProtocolVersion {
			t.Fatalf("ValidateDataRequest(%d) = %+v", version, err)
		}
	}
	if err := ValidateDataRequest(ProtocolVersionNotespaceID, ""); err == nil || err.Code != ErrorUnregisteredNotespace {
		t.Fatalf("missing id error = %+v", err)
	}
	if err := ValidateDataRequest(ProtocolVersionNotespaceID, "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err != nil {
		t.Fatalf("v3 id rejected: %v", err)
	}
	versions := SupportedProtocolVersions()
	if len(versions) != 3 || versions[0] != 3 || versions[1] != 2 || versions[2] != 1 {
		t.Fatalf("supported versions = %v, want [3 2 1]", versions)
	}
	// The existing v2 signing domain is deliberately unchanged by v3.
	if capabilitiesSignatureDomain != "grove.sync.capabilities.v2" {
		t.Fatalf("device-session signing domain changed: %q", capabilitiesSignatureDomain)
	}
}

func TestResolutionAndRerecordCarryReplayAndStaleGuards(t *testing.T) {
	identity := RequestIdentity{ProtocolVersion: 3, IdempotencyKey: "resolve-1", DeviceID: "device-1"}
	resolution := RegistrationResolutionRequest{RequestIdentity: identity, ConflictID: "conflict-1", SurvivorNotespaceID: "survivor", LoserNotespaceID: "loser", LoserDisposition: RegistrationIntentCreateSibling, ExpectedVersion: 7}
	rerecord := SubjectRerecordRequest{RequestIdentity: identity, NotespaceID: "survivor", OldSubject: "github.com/old/Core", NewSubject: "github.com/new/Core", ExpectedVersion: 8}
	for name, value := range map[string]any{"resolution": resolution, "rerecord": rerecord} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"\"idempotency_key\"", "\"device_id\"", "\"expected_version\""} {
			if !jsonContains(data, key) {
				t.Fatalf("%s missing %s: %s", name, key, data)
			}
		}
	}
}

func jsonContains(data []byte, fragment string) bool {
	for i := 0; i+len(fragment) <= len(data); i++ {
		if string(data[i:i+len(fragment)]) == fragment {
			return true
		}
	}
	return false
}
