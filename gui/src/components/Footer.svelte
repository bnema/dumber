<script lang="ts">
    const openGithub = () => {
        window.open("https://github.com/bnema/dumber", "_blank");
    };
</script>

<footer class="footer">
    <div class="footer-separator"></div>
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
        margin-top: 1rem;
        width: 100%;
    }

    .footer-separator {
        height: 2px;
        background: #404040;
    }

    .footer-content {
        display: flex;
        justify-content: center;
        align-items: center;
        padding: 1rem 0;
    }

    .footer-text {
        display: flex;
        align-items: center;
        gap: 0.75rem;
        font-size: 0.85rem;
        color: #888;
    }

    .footer-name {
        color: #aaa;
        font-weight: 500;
    }

    .footer-dot {
        color: #666;
    }

    .footer-link {
        background: none;
        border: none;
        color: #0066cc;
        text-decoration: none;
        cursor: pointer;
        font-size: 0.85rem;
        padding: 0;
        transition: color 0.2s ease;
    }

    .footer-link:hover {
        color: #4d94ff;
        text-decoration: underline;
    }

    .footer-link:focus {
        outline: 1px solid #0066cc;
        outline-offset: 2px;
        border-radius: 2px;
    }
</style>
