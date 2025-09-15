const CosmeticFilter = (() => {
    const hiddenElements = new WeakSet();
    const selectors = {
        generic: [],      // Apply everywhere
        specific: [],     // Domain-specific
        procedural: []    // Complex selectors with :has(), etc.
    };

    // Safe anti-breakage scriptlets that avoid WebKit internal interference
    const antiBreakage = {
        // Safely stub ad services without modifying global setTimeout
        neutralizeAdPromises() {
            try {
                // Use WeakMap to avoid direct property modification
                const stubServices = new WeakMap();

                // Only stub if not already defined to avoid conflicts
                if (typeof window.googletag === 'undefined') {
                    window.googletag = {
                        cmd: [],
                        push: () => {},
                        enableServices: () => {},
                        display: () => {},
                        defineSlot: () => ({ addService: () => {}, setTargeting: () => {} })
                    };
                }

                // Stub Google Analytics safely
                if (typeof window.gtag === 'undefined') {
                    window.gtag = function() {};
                }
                if (typeof window.ga === 'undefined') {
                    window.ga = function() {};
                }
            } catch (e) {
                // Fail silently to avoid breaking pages
                console.debug('[dumber] Anti-breakage initialization failed:', e);
            }
        },

        // Clean up loading indicators after page load
        cleanupLoadingIndicators() {
            // Wait for page to be more stable before cleanup
            const cleanup = () => {
                try {
                    // Only hide elements that are clearly loading indicators
                    const loadingSelectors = [
                        '.loading:not([role])', // Avoid hiding important loading states
                        '.spinner:not([aria-live])',
                        '.loader:not([aria-live])',
                        '[class*="ad-loading"]:not([aria-live])'
                    ];

                    loadingSelectors.forEach(selector => {
                        document.querySelectorAll(selector).forEach(el => {
                            // Check if element is actually a loading indicator
                            const rect = el.getBoundingClientRect();
                            if (rect.width > 0 && rect.height > 0) {
                                el.style.opacity = '0.3';
                                el.style.transition = 'opacity 0.5s';
                            }
                        });
                    });
                } catch (e) {
                    console.debug('[dumber] Loading cleanup failed:', e);
                }
            };

            // Defer cleanup to avoid interfering with page initialization
            if (document.readyState === 'complete') {
                setTimeout(cleanup, 2000);
            } else {
                window.addEventListener('load', () => setTimeout(cleanup, 2000));
            }
        }
    };

    // High-performance element hiding
    const hideElements = (elements) => {
        const styleId = 'dumber-cosmetic-style';
        let styleEl = document.getElementById(styleId);

        if (!styleEl) {
            styleEl = document.createElement('style');
            styleEl.id = styleId;
            styleEl.textContent = '';
            (document.head || document.documentElement).appendChild(styleEl);
        }

        // Batch CSS injection for performance
        const cssRules = elements.map(selector =>
            `${selector} { display: none !important; }`
        ).join('\n');

        styleEl.textContent += cssRules;
    };

    // Handle complex procedural selectors
    const applyProceduralFilters = () => {
        selectors.procedural.forEach(rule => {
            if (rule.includes(':has(')) {
                // Handle :has() pseudo-selector
                const match = rule.match(/(.+):has\((.+)\)/);
                if (match) {
                    const [_, parent, child] = match;
                    document.querySelectorAll(parent).forEach(el => {
                        if (el.querySelector(child)) {
                            el.style.display = 'none';
                            hiddenElements.add(el);
                        }
                    });
                }
            } else if (rule.includes(':not(')) {
                // Handle :not() pseudo-selector
                try {
                    document.querySelectorAll(rule).forEach(el => {
                        el.style.display = 'none';
                        hiddenElements.add(el);
                    });
                } catch (e) {
                    // Invalid selector, skip
                }
            }
        });
    };

    // MutationObserver for lazy-loaded content
    const observer = new MutationObserver((mutations) => {
        // Debounce for performance
        if (observer.timeout) {
            clearTimeout(observer.timeout);
        }

        observer.timeout = setTimeout(() => {
            // Check if new elements match our selectors
            const allSelectors = [...selectors.generic, ...selectors.specific];

            mutations.forEach(mutation => {
                // Handle added nodes
                mutation.addedNodes.forEach(node => {
                    if (node.nodeType !== 1) return; // Only element nodes

                    // Check the node itself
                    allSelectors.forEach(selector => {
                        if (node.matches && node.matches(selector)) {
                            node.style.display = 'none';
                            hiddenElements.add(node);
                        }
                    });

                    // Check descendants
                    if (node.querySelectorAll) {
                        allSelectors.forEach(selector => {
                            node.querySelectorAll(selector).forEach(el => {
                                el.style.display = 'none';
                                hiddenElements.add(el);
                            });
                        });
                    }
                });

                // Handle attribute changes (for dynamic ads)
                if (mutation.type === 'attributes') {
                    const target = mutation.target;
                    allSelectors.forEach(selector => {
                        if (target.matches && target.matches(selector)) {
                            target.style.display = 'none';
                            hiddenElements.add(target);
                        }
                    });
                }
            });

            // Reapply procedural filters
            applyProceduralFilters();
        }, 50); // 50ms debounce
    });

    // Initialize cosmetic filtering
    const init = (rules) => {
        // Initialize safe anti-breakage scriptlets
        antiBreakage.neutralizeAdPromises();
        antiBreakage.cleanupLoadingIndicators();

        // Parse and categorize rules (with null safety)
        if (rules && Array.isArray(rules)) {
            rules.forEach(rule => {
                if (rule.includes(':has(') || rule.includes(':not(')) {
                    selectors.procedural.push(rule);
                } else if (rule.domain === window.location.hostname) {
                    selectors.specific.push(rule.selector);
                } else {
                    selectors.generic.push(rule.selector);
                }
            });
        }

        // Apply initial hiding
        hideElements([...selectors.generic, ...selectors.specific]);
        applyProceduralFilters();

        // Start observing for changes
        observer.observe(document.documentElement, {
            childList: true,
            subtree: true,
            attributes: true,
            attributeFilter: ['class', 'id', 'style']
        });
    };

    // Performance optimization: batch rule updates
    const updateRules = (newRules) => {
        // Diff and apply only new rules
        const newSelectors = newRules.filter(r =>
            !selectors.generic.includes(r) &&
            !selectors.specific.includes(r)
        );

        if (newSelectors.length > 0) {
            hideElements(newSelectors);
            selectors.generic.push(...newSelectors);
        }
    };

    // Cleanup function for navigation
    const cleanup = () => {
        observer.disconnect();
        hiddenElements.clear();
        const styleEl = document.getElementById('dumber-cosmetic-style');
        if (styleEl) styleEl.remove();
    };

    return { init, updateRules, cleanup };
})();

// Inject rules from native
window.__dumber_cosmetic_init = (rules) => CosmeticFilter.init(rules);
window.__dumber_cosmetic_update = (rules) => CosmeticFilter.updateRules(rules);
window.__dumber_cosmetic_cleanup = () => CosmeticFilter.cleanup();