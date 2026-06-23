package webkit

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/puregotk/v4/webkit"
	"github.com/rs/zerolog"
)

func TestSettingsManagerUpdateFromPayloadStoresTypedRuntimeSettings(t *testing.T) {
	sm := NewSettingsManager(context.Background(), port.EngineSettingsPayload{})
	payload := port.EngineSettingsPayload{
		WebContent: port.EngineWebContentSettingsPayload{
			SansFont:         "Inter",
			SerifFont:        "Literata",
			MonospaceFont:    "Fira Code",
			DefaultFontSize:  16,
			HardwareDecoding: port.EngineHardwareDecodingForce,
		},
	}

	sm.UpdateFromPayload(context.Background(), payload)

	got := sm.current()
	if got.WebContent.SansFont != "Inter" ||
		got.WebContent.SerifFont != "Literata" ||
		got.WebContent.MonospaceFont != "Fira Code" ||
		got.WebContent.DefaultFontSize != 16 ||
		got.WebContent.HardwareDecoding != port.EngineHardwareDecodingForce {
		t.Fatalf("stored payload=%#v", got)
	}
}

func TestApplyMediaSettingsUpdatesHardwareDecodingKnobsOnSameSettingsObject(t *testing.T) {
	tests := []struct {
		name         string
		first        port.EngineHardwareDecodingMode
		next         port.EngineHardwareDecodingMode
		wantPolicy   webkit.HardwareAccelerationPolicy
		wantHWTypes  string
		wantEmptyHWT bool
	}{
		{
			name:        "disable to force resets acceleration policy",
			first:       port.EngineHardwareDecodingDisable,
			next:        port.EngineHardwareDecodingForce,
			wantPolicy:  webkit.HardwareAccelerationPolicyAlwaysValue,
			wantHWTypes: "video/av01;video/mp4;video/webm;video/x-h264;video/x-h265",
		},
		{
			name:         "force to disable clears required content types",
			first:        port.EngineHardwareDecodingForce,
			next:         port.EngineHardwareDecodingDisable,
			wantPolicy:   webkit.HardwareAccelerationPolicyNeverValue,
			wantEmptyHWT: true,
		},
		{
			name:         "disable to auto resets acceleration policy",
			first:        port.EngineHardwareDecodingDisable,
			next:         port.EngineHardwareDecodingAuto,
			wantPolicy:   webkit.HardwareAccelerationPolicyAlwaysValue,
			wantEmptyHWT: true,
		},
	}

	logger := zerolog.Nop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &recordingMediaSettings{}

			applyMediaSettings(settings, tt.first, &logger)
			applyMediaSettings(settings, tt.next, &logger)

			if got := settings.hardwareAccelerationPolicy; got != tt.wantPolicy {
				t.Fatalf("HardwareAccelerationPolicy=%v, want %v", got, tt.wantPolicy)
			}
			gotHWTypes := settings.mediaContentTypesRequiringHardwareSupport
			if tt.wantEmptyHWT {
				if gotHWTypes != "" {
					t.Fatalf("MediaContentTypesRequiringHardwareSupport=%q, want empty", gotHWTypes)
				}
				return
			}
			if gotHWTypes != tt.wantHWTypes {
				t.Fatalf("MediaContentTypesRequiringHardwareSupport=%q, want %q", gotHWTypes, tt.wantHWTypes)
			}
		})
	}
}

type recordingMediaSettings struct {
	hardwareAccelerationPolicy                webkit.HardwareAccelerationPolicy
	mediaContentTypesRequiringHardwareSupport string
}

func (*recordingMediaSettings) SetEnableWebaudio(bool) {}

func (*recordingMediaSettings) SetEnableWebgl(bool) {}

func (*recordingMediaSettings) SetEnableMedia(bool) {}

func (*recordingMediaSettings) SetEnableMediasource(bool) {}

func (*recordingMediaSettings) SetEnableMediaCapabilities(bool) {}

func (*recordingMediaSettings) SetEnableEncryptedMedia(bool) {}

func (*recordingMediaSettings) SetMediaPlaybackRequiresUserGesture(bool) {}

func (*recordingMediaSettings) SetMediaPlaybackAllowsInline(bool) {}

func (s *recordingMediaSettings) SetHardwareAccelerationPolicy(policy webkit.HardwareAccelerationPolicy) {
	s.hardwareAccelerationPolicy = policy
}

func (s *recordingMediaSettings) SetMediaContentTypesRequiringHardwareSupport(contentTypes *string) {
	if contentTypes == nil {
		s.mediaContentTypesRequiringHardwareSupport = ""
		return
	}
	s.mediaContentTypesRequiringHardwareSupport = *contentTypes
}
