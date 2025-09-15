/**
 * Find in Page Functionality
 *
 * Text search and highlighting implementation
 */

import type { FindMatch, HighlightNode } from './types';
import { omniboxStore } from './stores.svelte.ts';

/**
 * Find text in the current page and highlight matches
 */
export function findInPage(query: string): void {
  // Clear previous highlights
  omniboxStore.clearHighlights();

  const trimmedQuery = (query || '').trim();
  if (!trimmedQuery) {
    return;
  }

  const body = document.body;
  if (!body) {
    return;
  }

  // Get omnibox root to exclude from search
  const omniboxRoot = document.getElementById('dumber-omnibox-root');
  const matches: FindMatch[] = [];
  const maxMatches = omniboxStore.config.maxMatches;

  // Create tree walker to find text nodes
  const walker = document.createTreeWalker(
    body,
    NodeFilter.SHOW_TEXT,
    {
      acceptNode(node: Text): number {
        // Skip empty or whitespace-only nodes
        if (!node.nodeValue || !node.nodeValue.trim()) {
          return NodeFilter.FILTER_REJECT;
        }

        const parentElement = node.parentElement;
        if (!parentElement) {
          return NodeFilter.FILTER_REJECT;
        }

        // Skip omnibox content
        if (omniboxRoot && omniboxRoot.contains(parentElement)) {
          return NodeFilter.FILTER_REJECT;
        }

        // Skip script, style, and noscript tags
        const tagName = parentElement.tagName;
        if (tagName === 'SCRIPT' || tagName === 'STYLE' || tagName === 'NOSCRIPT') {
          return NodeFilter.FILTER_REJECT;
        }

        // Skip hidden elements
        const computedStyle = getComputedStyle(parentElement);
        if (computedStyle.visibility === 'hidden' || computedStyle.display === 'none') {
          return NodeFilter.FILTER_REJECT;
        }

        return NodeFilter.FILTER_ACCEPT;
      }
    }
  );

  const queryLowerCase = trimmedQuery.toLowerCase();

  // Process each text node
  while (walker.nextNode() && matches.length < maxMatches) {
    const textNode = walker.currentNode as Text;
    let nodeValue = textNode.nodeValue || '';
    let searchIndex = 0;

    // Find all matches in this text node
    while (nodeValue && matches.length < maxMatches) {
      const matchIndex = nodeValue.toLowerCase().indexOf(queryLowerCase, searchIndex);
      if (matchIndex === -1) {
        break;
      }

      // Create text nodes for before, match, and after
      const beforeText = nodeValue.slice(0, matchIndex);
      const matchText = nodeValue.slice(matchIndex, matchIndex + trimmedQuery.length);
      const afterText = nodeValue.slice(matchIndex + trimmedQuery.length);

      // Create elements
      const beforeNode = document.createTextNode(beforeText);
      const matchTextNode = document.createTextNode(matchText);
      const highlightSpan = document.createElement('span');

      // Style the highlight span
      highlightSpan.className = 'find-highlight';
      highlightSpan.appendChild(matchTextNode);

      // Insert into DOM
      const parent = textNode.parentNode;
      if (!parent) {
        break;
      }

      parent.insertBefore(beforeNode, textNode);
      parent.insertBefore(highlightSpan, textNode);
      textNode.nodeValue = afterText;

      // Track highlight node for cleanup
      const highlightNode: HighlightNode = {
        span: highlightSpan,
        text: matchTextNode
      };
      omniboxStore.addHighlightNode(highlightNode);

      // Create match context
      const leftContext = beforeText.slice(-30);
      let rightContext = afterText.slice(0, 30);

      // Try to break at word boundaries
      const boundaryMatch = rightContext.match(/[.,;:-]/);
      if (boundaryMatch && boundaryMatch.index !== undefined) {
        rightContext = rightContext.slice(0, boundaryMatch.index + 1);
      }

      const context = (leftContext + matchText + rightContext)
        .replace(/\s+/g, ' ')
        .trim();

      // Add to matches
      const findMatch: FindMatch = {
        element: highlightSpan,
        context
      };
      matches.push(findMatch);

      // Continue searching in the remaining text
      nodeValue = afterText;
      searchIndex = 0;
    }
  }

  // Update store with matches
  omniboxStore.updateMatches(matches);

  // Reveal first match if any
  if (matches.length > 0) {
    revealMatch(0);
  }
}

/**
 * Reveal a specific match by index
 */
export function revealMatch(index: number): void {
  const matches = omniboxStore.matches;
  if (index < 0 || index >= matches.length) {
    return;
  }

  // Remove previous active class
  const prevActiveIndex = omniboxStore.activeIndex;
  if (prevActiveIndex >= 0 && matches[prevActiveIndex]) {
    const prevElement = matches[prevActiveIndex].element;
    if (prevElement.classList) {
      prevElement.classList.remove('find-active');
    }
  }

  // Add active class to current match
  const currentMatch = matches[index];
  if (currentMatch && currentMatch.element.classList) {
    currentMatch.element.classList.add('find-active');
    omniboxStore.setActiveIndex(index);

    // Scroll into view
    try {
      currentMatch.element.scrollIntoView({
        block: 'center',
        inline: 'nearest'
      });
    } catch {
      currentMatch.element.scrollIntoView();
    }
  }
}

/**
 * Jump to next/previous match
 */
export function jumpToMatch(delta: number): void {
  const matches = omniboxStore.matches;
  if (matches.length === 0) {
    return;
  }

  const currentIndex = omniboxStore.selectedIndex || 0;
  const newIndex = ((currentIndex + delta) % matches.length + matches.length) % matches.length;

  omniboxStore.setSelectedIndex(newIndex);
  revealMatch(newIndex);
}