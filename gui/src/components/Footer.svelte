<script lang="ts">
    const openGithub = () => {
        window.open("https://github.com/bnema/dumber", "_blank");
    };
</script>

<footer class="footer">
    <div class="footer-content">
        <div class="footer-text">
            <span class="footer-name">Dumber Browser</span>
            <span class="footer-dot">•</span>
            <button class="footer-link" onclick={openGithub}>
                View on GitHub
            </button>
        </div>
    </div>
</footer>

<style>
    .footer {
        margin-top: 1.5rem;
        width: 100%;
        border-top: 1px solid var(--dynamic-border);
        background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
    }
    
    .footer-content {
        display: flex;
        justify-content: center;
        align-items: center;
        padding: 0.75rem 0;
    }

    .footer-text {
        display: flex;
        align-items: center;
        gap: 0.75rem;
        font-size: 0.75rem;
        text-transform: uppercase;
        letter-spacing: 0.12em;
        color: var(--dynamic-muted);
    }

    .footer-name {
        color: var(--dynamic-text);
        font-weight: 600;
    }

    .footer-dot {
        color: var(--dynamic-border);
    }

    .footer-link {
        background: none;
        border: none;
        color: var(--dynamic-muted);
        text-decoration: none;
        cursor: pointer;
        font-size: 0.85rem;
        padding: 0;
        transition: color 0.2s ease, letter-spacing 0.2s ease;
        font-family:
            "JetBrains Mono",
            "Fira Code",
            "SFMono-Regular",
            Menlo,
            monospace;
    }

    .footer-link:hover {
        color: var(--dynamic-text);
        letter-spacing: 0.16em;
    }

    .footer-link:focus {
        outline: 1px solid var(--dynamic-border);
        outline-offset: 2px;
        border-radius: 0;
    }
</style>
