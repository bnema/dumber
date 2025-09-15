package webkit

import (
	"strings"
)

// GenerateCodecControlScript creates JavaScript code to control codec preferences
// This script overrides MediaCapabilities API and platform-specific player configurations
func GenerateCodecControlScript(prefs CodecPreferencesConfig) string {
	// Build blocked codecs regex pattern
	var blockedCodecs []string
	for _, codec := range prefs.BlockedCodecs {
		switch strings.ToLower(codec) {
		case "vp9":
			blockedCodecs = append(blockedCodecs, "vp9", "vp09")
		case "vp8":
			blockedCodecs = append(blockedCodecs, "vp8", "vp08")
		case "h264":
			blockedCodecs = append(blockedCodecs, "h264", "avc1")
		case "h265", "hevc":
			blockedCodecs = append(blockedCodecs, "h265", "hevc", "hev1", "hvc1")
		default:
			blockedCodecs = append(blockedCodecs, strings.ToLower(codec))
		}
	}

	// Build preferred codecs list
	var preferredCodecs []string
	for _, codec := range prefs.PreferredCodecs {
		switch strings.ToLower(codec) {
		case "av1":
			preferredCodecs = append(preferredCodecs, "av01", "av1")
		case "h264":
			preferredCodecs = append(preferredCodecs, "h264", "avc1")
		case "vp9":
			preferredCodecs = append(preferredCodecs, "vp9", "vp09")
		case "vp8":
			preferredCodecs = append(preferredCodecs, "vp8", "vp08")
		default:
			preferredCodecs = append(preferredCodecs, strings.ToLower(codec))
		}
	}

	blockedPattern := strings.Join(blockedCodecs, "|")
	hasBlockedCodecs := len(blockedCodecs) > 0
	hasPreferredCodecs := len(preferredCodecs) > 0

	// Convert preferredCodecs slice to proper JavaScript array syntax
	preferredCodecsJS := "[" + strings.Join(func() []string {
		var quoted []string
		for _, codec := range preferredCodecs {
			quoted = append(quoted, `"`+codec+`"`)
		}
		return quoted
	}(), ", ") + "]"

	// Build JavaScript directly without format specifiers
	var js strings.Builder

	js.WriteString(`(() => {
    'use strict';

    const isMainFrame = (window.self === window.top);
    function log(msg) {
        if (isMainFrame) console.log('[dumber-codec] ' + msg);
    }

    log('Initializing codec control');
    
    // Smart fullscreen handling that works with WebKit's native behavior
    const videoFullscreenStates = new WeakMap();
    let fullscreenTransitionInProgress = false;
    
    // Minimal state tracking - only what's essential
    function trackVideoForFullscreen(video) {
        if (!video || video.tagName !== 'VIDEO') return;
        
        // Only track if video has meaningful content
        if (video.readyState < 2 || !video.duration) return;
        
        const state = {
            wasPlaying: !video.paused && !video.ended,
            currentTime: video.currentTime,
            playbackRate: video.playbackRate
        };
        
        videoFullscreenStates.set(video, state);
        log('Tracking video for fullscreen - playing: ' + state.wasPlaying + ', time: ' + state.currentTime);
    }
    
    // Gentle recovery without forcing reloads
    function recoverVideoAfterFullscreen(video) {
        const state = videoFullscreenStates.get(video);
        if (!state) return;
        
        log('Recovering video after fullscreen transition');
        
        // Only intervene if video is stuck in a loading state
        const isStuck = video.readyState < 2 && video.networkState === 2; // NETWORK_LOADING but not ready
        const hasLostTime = Math.abs(video.currentTime - state.currentTime) > 2;
        
        if (isStuck || hasLostTime) {
            log('Video appears stuck, applying gentle recovery');
            
            // Gentle nudge - just try to resume if it was playing
            if (state.wasPlaying && video.paused) {
                setTimeout(() => {
                    if (video.readyState >= 2) {
                        video.play().catch(e => log('Play failed: ' + e));
                    }
                }, 200);
            }
        }
        
        videoFullscreenStates.delete(video);
    }
    
    // WebKit-aware fullscreen change detection  
    function handleWebKitFullscreenTransition() {
        if (fullscreenTransitionInProgress) return;
        fullscreenTransitionInProgress = true;
        
        log('WebKit fullscreen transition detected');
        
        // Skip fullscreen recovery on Twitch to prevent theater/fullscreen freezing
        if (location.hostname.includes('twitch.tv')) {
            log('Skipping fullscreen recovery on Twitch for stability');
            fullscreenTransitionInProgress = false;
            return;
        }
        
        const videos = document.querySelectorAll('video');
        const isEntering = !!(document.fullscreenElement || document.webkitFullscreenElement);
        
        videos.forEach(video => {
            if (isEntering) {
                trackVideoForFullscreen(video);
            } else {
                // Give WebKit time to complete its native fullscreen handling
                setTimeout(() => recoverVideoAfterFullscreen(video), 500);
            }
        });
        
        setTimeout(() => {
            fullscreenTransitionInProgress = false;
        }, 1000);
    }
    
    // Use passive listeners to avoid interfering with WebKit's native handling
    document.addEventListener('fullscreenchange', handleWebKitFullscreenTransition, { passive: true });
    document.addEventListener('webkitfullscreenchange', handleWebKitFullscreenTransition, { passive: true });
    
    // Override canPlayType for HTMLMediaElement (primary codec detection)
    const originalCanPlayType = HTMLMediaElement.prototype.canPlayType;
    HTMLMediaElement.prototype.canPlayType = function(type) {
        log('canPlayType called with: ' + type);
        
        // Block unwanted codecs`)

	if hasBlockedCodecs {
		js.WriteString(`
        const blockedRegex = /` + blockedPattern + `/i;
        if (blockedRegex.test(type)) {
            log('Blocking codec via canPlayType: ' + type);
            return '';
        }`)
	}

	if hasPreferredCodecs {
		js.WriteString(`
        
        // Boost preferred codecs
        const preferredCodecs = ` + preferredCodecsJS + `;
        for (const preferred of preferredCodecs) {
            if (type.toLowerCase().includes(preferred)) {
                log('Boosting preferred codec via canPlayType: ' + type);
                return 'probably';
            }
        }`)
	}

	js.WriteString(`
        
        // Call original for other codecs
        return originalCanPlayType.call(this, type);
    };
    
    // Override MediaSource.isTypeSupported for more aggressive codec blocking
    if (window.MediaSource && MediaSource.isTypeSupported) {
        const originalIsTypeSupported = MediaSource.isTypeSupported.bind(MediaSource);
        MediaSource.isTypeSupported = function(type) {
            log('MediaSource.isTypeSupported called with: ' + type);`)

	if hasBlockedCodecs {
		js.WriteString(`
            
            // Block unwanted codecs
            const blockedRegex = /` + blockedPattern + `/i;
            if (blockedRegex.test(type)) {
                log('Blocking codec via MediaSource.isTypeSupported: ' + type);
                return false;
            }`)
	}

	if hasPreferredCodecs {
		js.WriteString(`
            
            // Boost preferred codecs
            const preferredCodecs = ` + preferredCodecsJS + `;
            for (const preferred of preferredCodecs) {
                if (type.toLowerCase().includes(preferred)) {
                    log('Boosting preferred codec via MediaSource.isTypeSupported: ' + type);
                    return true;
                }
            }`)
	}

	js.WriteString(`
            
            // Call original for other codecs
            return originalIsTypeSupported(type);
        };
    }
    
    // Override MediaCapabilities API
    if (navigator.mediaCapabilities && navigator.mediaCapabilities.decodingInfo) {
        const originalDecodingInfo = navigator.mediaCapabilities.decodingInfo.bind(navigator.mediaCapabilities);
        
        navigator.mediaCapabilities.decodingInfo = async function(config) {
            const contentType = config.video?.contentType || config.type || '';
            
            // Parse resolution from video config for resolution-aware decisions
            let width = 0, height = 0;
            if (config.video) {
                width = config.video.width || 0;
                height = config.video.height || 0;
            }
            
            // Determine if this is a high resolution request
            const isHighRes = height >= 1440 || width >= 2560; // 1440p/1080p ultrawide+
            const is4K = height >= 2160 || width >= 3840;
            const is1080p60 = (height >= 1080 || width >= 1920) && 
                             (config.video?.framerate >= 50 || config.video?.frameRate >= 50);
            
            log('MediaCapabilities query - contentType: ' + contentType +
                       ', resolution: ' + width + 'x' + height + ', highRes: ' + isHighRes + ', 4K: ' + is4K + ', 1080p60: ' + is1080p60);
            
            // Block unwanted codecs`)

	if hasBlockedCodecs {
		js.WriteString(`
            const blockedRegex = /` + blockedPattern + `/i;
            if (blockedRegex.test(contentType)) {
                log('Blocking codec: ' + contentType);
                return {
                    supported: false,
                    smooth: false,
                    powerEfficient: false
                };
            }`)
	}

	js.WriteString(`
            
            // Resolution-aware preferred codec handling`)

	if hasPreferredCodecs {
		js.WriteString(`
            const preferredCodecs = ` + preferredCodecsJS + `;
            for (const preferred of preferredCodecs) {
                if (contentType.toLowerCase().includes(preferred)) {
                    log('Boosting preferred codec: ' + contentType);
                    
                    // Resolution-aware capability reporting for AV1
                    if (preferred === 'av01' || preferred === 'av1') {
                        // AV1 handling based on resolution
                        if (is4K) {
                            // 4K AV1: supported but not smooth (encourage VP9 fallback)
                            return {
                                supported: true,
                                smooth: false,
                                powerEfficient: false
                            };
                        } else if (is1080p60) {
                            // 1080p60 AV1: supported but not power efficient
                            return {
                                supported: true,
                                smooth: true,
                                powerEfficient: false
                            };
                        } else if (isHighRes) {
                            // 1440p AV1: supported but not optimal
                            return {
                                supported: true,
                                smooth: false,
                                powerEfficient: true
                            };
                        } else {
                            // <= 1080p30 AV1: fully supported
                            return {
                                supported: true,
                                smooth: true,
                                powerEfficient: true
                            };
                        }
                    } else {
                        // Other preferred codecs (VP9, H264) - always report as capable
                        return {
                            supported: true,
                            smooth: true,
                            powerEfficient: true
                        };
                    }
                }
            }`)
	}

	js.WriteString(`
            
            // Call original for other codecs
            try {
                return await originalDecodingInfo(config);
            } catch (e) {
                console.warn('[dumber-codec] MediaCapabilities error:', e);
                return {
                    supported: false,
                    smooth: false,
                    powerEfficient: false
                };
            }
        };
    }
    
    // YouTube-specific codec forcing
    if (location.hostname.includes('youtube.com')) {
        log('Applying YouTube codec preferences');
        
        // Set YouTube's localStorage preferences for smart AV1 usage
        // 2048 = prefer AV1 for lower resolutions, fallback to VP9 for higher res
        // This allows YouTube to make smarter codec decisions based on our MediaCapabilities
        try {
            localStorage.setItem('yt-player-av1-pref', '2048');
            Object.defineProperty(localStorage, 'yt-player-av1-pref', {
                value: '2048',
                writable: false,
                configurable: false
            });
            log('YouTube: Set smart AV1 localStorage preference (2048)');
        } catch (e) {
            console.warn('[dumber-codec] YouTube: Failed to set localStorage:', e);
        }
        
        // Override ytInitialPlayerResponse parsing
        Object.defineProperty(window, 'ytplayer', {
            configurable: true,
            get() {
                return this._ytplayer || null;
            },
            set(player) {
                if (player && player.config && player.config.args) {`)

	if prefs.ForceAV1 {
		js.WriteString(`
                    // Force AV1 if enabled
                    player.config.args.preferred_codecs = 'av01';
                    log('YouTube: Forced AV1 codec');`)
	}

	if hasPreferredCodecs {
		js.WriteString(`
                    // Set codec preference order
                    const preferredCodecs = ` + preferredCodecsJS + `;
                    const codecOrder = preferredCodecs.map(c => {
                        switch(c.toLowerCase()) {
                            case 'av1': case 'av01': return 'av01';
                            case 'h264': case 'avc1': return 'avc1';
                            case 'vp9': case 'vp09': return 'vp9';
                            case 'vp8': case 'vp08': return 'vp8';
                            default: return c;
                        }
                    }).join(',');
                    player.config.args.preferred_codecs = codecOrder;
                    log('YouTube: Set codec order: ' + codecOrder);`)
	}

	js.WriteString(`
                }
                this._ytplayer = player;
            }
        });
        
        // Intercept YouTube's format selection
        const originalFetch = window.fetch;
        window.fetch = function(input, init) {
            if (typeof input === 'string' && input.includes('/youtubei/v1/player')) {
                log('YouTube: Intercepting player request');`)

	if hasPreferredCodecs {
		js.WriteString(`
                
                // Modify request to prefer our codecs
                if (init && init.body) {
                    try {
                        const body = JSON.parse(init.body);
                        if (body.videoId) {
                            // Add codec preferences to the request
                            body.codecPreferences = ` + preferredCodecsJS + `;
                            init.body = JSON.stringify(body);
                        }
                    } catch (e) {
                        // Non-JSON body, ignore
                    }
                }`)
	}

	js.WriteString(`
            }
            
            // Process YouTube player API responses for format manipulation
            if (typeof input === 'string' && input.includes('/youtubei/v1/player')) {
                return originalFetch.call(this, input, init).then(response => {
                    if (!response.ok) return response;
                    
                    return response.text().then(text => {
                        try {
                            const data = JSON.parse(text);
                            
                            // Manipulate streaming data if present
                            if (data.streamingData && data.streamingData.formats) {
                                log('YouTube: Processing format manifest');
                                
                                // Sort formats to prioritize AV1 for lower resolutions, VP9 for higher
                                data.streamingData.formats.sort((a, b) => {
                                    const aHeight = a.height || 0;
                                    const bHeight = b.height || 0;
                                    const aIsAV1 = a.mimeType && a.mimeType.includes('av01');
                                    const bIsAV1 = b.mimeType && b.mimeType.includes('av01');
                                    const aIsVP9 = a.mimeType && a.mimeType.includes('vp9');
                                    const bIsVP9 = b.mimeType && b.mimeType.includes('vp9');
                                    
                                    // For resolutions <= 1080p, prioritize AV1
                                    if (Math.max(aHeight, bHeight) <= 1080) {
                                        if (aIsAV1 && !bIsAV1) return -1;
                                        if (!aIsAV1 && bIsAV1) return 1;
                                    }
                                    // For resolutions > 1080p, prioritize VP9
                                    else {
                                        if (aIsVP9 && !bIsVP9) return -1;
                                        if (!aIsVP9 && bIsVP9) return 1;
                                    }
                                    
                                    return 0;
                                });
                                
                                log('YouTube: Reordered formats for optimal codec selection');
                            }
                            
                            return new Response(JSON.stringify(data), {
                                status: response.status,
                                statusText: response.statusText,
                                headers: response.headers
                            });
                        } catch (e) {
                            console.warn('[dumber-codec] YouTube: Failed to process response:', e);
                            return new Response(text, {
                                status: response.status,
                                statusText: response.statusText,
                                headers: response.headers
                            });
                        }
                    });
                });
            }
            
            return originalFetch.call(this, input, init);
        };
    }
    
    // Twitch: No codec interference - let Twitch handle codec selection natively
    if (location.hostname.includes('twitch.tv')) {
        log('Detected Twitch domain - no codec interference for stability');
        // Twitch codec control completely removed to prevent theater/fullscreen freezing
    }
    
    // Passive video monitoring without interfering with element creation
    const monitorExistingVideos = () => {
        const videos = document.querySelectorAll('video');
        videos.forEach(video => {
            if (!video.__codecMonitored) {
                video.__codecMonitored = true;
                log('Found video element, adding passive monitoring');
                
                // Passive codec detection only
                video.addEventListener('canplay', function() {
                    log('Video can play - resolution: ' + this.videoWidth + 'x' + this.videoHeight);
                }, { passive: true, once: true });
            }
        });
    };
    
    // Monitor for video elements periodically without overriding createElement
    const videoObserver = new MutationObserver(monitorExistingVideos);

    // Wait for document.body to be available before observing
    function startVideoObserver() {
        if (document.body) {
            videoObserver.observe(document.body, { childList: true, subtree: true });
            monitorExistingVideos(); // Check existing videos
            log('Video observer started');
        } else {
            // Retry after a short delay if body not ready
            setTimeout(startVideoObserver, 50);
        }
    }
    startVideoObserver();
    
    log('Codec control initialization complete');
})();`)

	return js.String()
}
