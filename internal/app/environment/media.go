package environment

import (
	"log"
	"os"
)

// SetupMediaPipeline proactively hardens media pipeline to avoid WebProcess crashes from buggy VAAPI/plugins.
// Can be disabled with DUMBER_MEDIA_SAFE=0
func SetupMediaPipeline() {
	if os.Getenv("DUMBER_MEDIA_SAFE") != "0" {
		// Demote VAAPI-related elements so they won't be auto-picked
		if os.Getenv("GST_PLUGIN_FEATURE_RANK") == "" {
			if err := os.Setenv("GST_PLUGIN_FEATURE_RANK", "vaapisink:0,vaapidecodebin:0,vaapih264dec:0,vaapivideoconvert:0"); err != nil {
				log.Printf("Warning: failed to set GST_PLUGIN_FEATURE_RANK: %v", err)
			}
		}
		// Optionally disable media stream features that can engage complex pipelines (webcam/mic)
		if os.Getenv("WEBKIT_DISABLE_MEDIA_STREAM") == "" {
			if err := os.Setenv("WEBKIT_DISABLE_MEDIA_STREAM", "1"); err != nil {
				log.Printf("Warning: failed to set WEBKIT_DISABLE_MEDIA_STREAM: %v", err)
			}
		}
		log.Printf("[media] Safe mode enabled: VAAPI demoted, webcam/mic disabled (override with DUMBER_MEDIA_SAFE=0)")
	}
}
