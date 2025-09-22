import type { Plugin } from "vite";
import { writeFileSync, readFileSync } from "fs";
import { resolve } from "path";

interface PageConfig {
  /** Page name (used for filename and script name) */
  name: string;
  /** Page title */
  title: string;
  /** Script filename (e.g., 'homepage.min.js') */
  script: string;
  /** Optional CSS filename */
  css?: string;
  /** Target filename (defaults to name + '.html') */
  filename?: string;
}

// Load the favicon SVG from assets
function getFaviconSVG(): string {
  const logoPath = resolve(__dirname, "..", "assets", "logo.svg");
  return readFileSync(logoPath, "utf-8");
}

// Create favicon data URL for inline usage
function getFaviconDataURL(): string {
  const svg = getFaviconSVG();
  const encoded = encodeURIComponent(svg)
    .replace(/'/g, "%27")
    .replace(/\(/g, "%28")
    .replace(/\)/g, "%29");
  return `data:image/svg+xml;utf8,${encoded}`;
}

// Generate HTML content for a page
function generatePageHTML(config: PageConfig): string {
  const cssLink = config.css
    ? `<link rel="stylesheet" href="./${config.css}">`
    : "";

  return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${config.title}</title>
    <link rel="icon" type="image/svg+xml" href="${getFaviconDataURL()}">
    ${cssLink}
</head>
<body>
    <script src="./${config.script}"></script>
</body>
</html>`;
}

/**
 * Vite plugin to generate dumb:// protocol pages
 * Supports multiple pages like homepage, config, about, etc.
 */
export function pageGenerator(pages: PageConfig[]): Plugin {
  return {
    name: "dumb-page-generator",
    writeBundle() {
      const assetsDir = resolve(__dirname, "..", "assets", "gui");

      try {
        // Generate HTML files for each page
        for (const page of pages) {
          const htmlContent = generatePageHTML(page);
          const filename = page.filename || `${page.name}.html`;
          writeFileSync(resolve(assetsDir, filename), htmlContent);
          console.log(`✓ Generated ${filename} for dumb://${page.name}`);
        }

        // Generate favicon.svg for scheme handler lookups
        const faviconContent = getFaviconSVG();
        writeFileSync(resolve(assetsDir, "favicon.svg"), faviconContent);
        console.log("✓ Generated favicon.svg");
      } catch (error) {
        console.error("✗ Failed to generate page files:", error);
        throw error;
      }
    },
  };
}

/**
 * Convenience function for homepage generation
 * @deprecated Use pageGenerator with homepage config instead
 */
export function homepageGenerator(): Plugin {
  return pageGenerator([
    {
      name: "homepage",
      title: "Dumber Browser",
      script: "homepage.min.js",
      filename: "index.html", // Special case: homepage uses index.html
    },
  ]);
}
