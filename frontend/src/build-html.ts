import { writeFileSync } from 'fs';
import { join } from 'path';
import { HTMLBuilder } from './utils/html-builder.js';

// Generate the HTML file
function buildHTML() {
  try {
    const html = HTMLBuilder.createBaseHTML();
    const outputPath = join(process.cwd(), 'dist', 'index.html');
    writeFileSync(outputPath, html);
    console.log('✓ Generated index.html');
  } catch (error) {
    console.error('✗ Failed to generate HTML:', error);
    process.exit(1);
  }
}

// Only run when called directly (not imported)
if (import.meta.url === `file://${process.argv[1]}`) {
  buildHTML();
}