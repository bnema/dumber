//go:build webkit_cgo

package webkit

// getOmniboxScript returns the injected omnibox/find component script.
// Kept in a cgo-tagged file to ensure availability when referenced
// from other cgo files in this package.
func getOmniboxScript() string {
    return `(() => {
  try {
    if (window.__dumber_omnibox_loaded) return; // idempotent
    window.__dumber_omnibox_loaded = true;

    const MAX_MATCHES = 2000;
    const H = {
      el: null,
      box: null,
      input: null,
      list: null,
      visible: false,
      mode: 'omnibox', // 'omnibox' | 'find'
      suggestions: [],
      matches: [], // {el, context}
      selectedIndex: -1,
      activeIndex: -1,
      debounceTimer: 0,
      highlightNodes: [],
      faded: false,
      prevOverflow: '',
      post(msg){ try { window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify(msg)); } catch(_){} },
      render(){
        if (!H.el) H.mount();
        H.el.style.display = H.visible ? 'block' : 'none';
        if (!H.visible) return;
        H.input.placeholder = H.mode === 'find' ? 'Find in page…' : 'Type URL or search…';
        H.input.focus();
      },
      setMode(m){ H.mode = m === 'find' ? 'find' : 'omnibox'; H.selectedIndex = -1; H.setFaded(false); H.paintList(); },
      open(mode, initial){ H.setMode(mode||'omnibox'); H.toggle(true); if (typeof initial==='string') { H.input.value = initial; H.onInput(); } },
      close(){ if (H.mode==='find') H.clearHighlights(); H.setFaded(false); H.toggle(false); },
      toggle(v){
        const newState = (typeof v==='boolean')? v : !H.visible;
        if (newState && !H.visible) {
          // Disable page scrolling while overlay is visible
          H.prevOverflow = document.documentElement.style.overflow;
          document.documentElement.style.overflow = 'hidden';
        } else if (!newState && H.visible) {
          // Restore page scrolling on close
          document.documentElement.style.overflow = H.prevOverflow || '';
          H.prevOverflow = '';
        }
        H.visible = newState;
        H.render();
      },
      setFaded(v){
        H.faded = !!v;
        if (!H.box || !H.input || !H.list) return;
        if (H.faded) {
          H.box.style.background = 'rgba(27,27,27,0.25)';
          H.box.style.backdropFilter = 'blur(2px) saturate(110%)';
          H.box.style.webkitBackdropFilter = 'blur(2px) saturate(110%)';
          H.input.style.background = 'rgba(18,18,18,0.35)';
          H.input.style.color = '#eee';
        } else {
          H.box.style.background = '#1b1b1b';
          H.box.style.backdropFilter = '';
          H.box.style.webkitBackdropFilter = '';
          H.input.style.background = '#121212';
          H.input.style.color = '#eee';
        }
      },
      scrollListToSelection(){
        if (!H.list) return;
        let idx = H.selectedIndex|0;
        if (idx < 0) return;
        // Account for header in find mode
        const childIdx = (H.mode==='find' ? 1 : 0) + idx;
        const el = H.list.children[childIdx];
        if (el && el.scrollIntoView) {
          try { el.scrollIntoView({block:'nearest'}); } catch(_) { el.scrollIntoView(); }
        }
      },
      mount(){
        const root = document.createElement('div');
        root.id = 'dumber-omnibox-root';
        root.style.cssText = 'position:fixed;inset:0;z-index:2147483647;display:none;';
        const box = document.createElement('div');
        box.style.cssText = 'max-width:720px;margin:8vh auto;padding:8px 10px;background:#1b1b1b;color:#eee;border:1px solid #444;border-radius:8px;box-shadow:0 10px 30px rgba(0,0,0,.6);font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,"Helvetica Neue",Arial,sans-serif;';
        const input = document.createElement('input');
        input.type = 'text';
        input.placeholder = 'Type URL or search…';
        input.style.cssText = 'width:100%;padding:10px 12px;border-radius:6px;border:1px solid #555;background:#121212;color:#eee;font-size:16px;outline:none;';
        const list = document.createElement('div');
        list.style.cssText = 'margin-top:8px;max-height:50vh;overflow:auto;border-top:1px solid #333;';
        const style = document.createElement('style');
        style.textContent = '.dumber-find-highlight{background:#ffeb3b;color:#000;padding:0 1px;border-radius:2px;box-shadow:0 0 0 1px #c8b900 inset}.dumber-find-active{background:#ff9800 !important;color:#000;box-shadow:0 0 0 1px #b36b00 inset}';
        document.documentElement.appendChild(style);
        box.appendChild(input); box.appendChild(list); root.appendChild(box); document.documentElement.appendChild(root);
        H.box = box;
        // Click outside the box closes the overlay
        root.addEventListener('mousedown', (e)=>{
          const tgt = e.target;
          if (tgt && box && !box.contains(tgt)) {
            e.preventDefault();
            e.stopPropagation();
            H.close();
          }
        }, true);
        box.addEventListener('mousedown', (e)=>{ e.stopPropagation(); }, true);
        // Focus and select input on hover for quick typing
        input.addEventListener('mouseenter', ()=>{ if (H.visible) { try { input.focus({preventScroll:true}); input.select(); } catch(_) { input.focus(); } } });
        root.addEventListener('mouseenter', ()=>{ if (H.visible) { try { input.focus({preventScroll:true}); } catch(_) { input.focus(); } H.setFaded(false); } });
        // Clear fading when user goes back to input typing
        input.addEventListener('focus', ()=> H.setFaded(false));
        input.addEventListener('keydown', (e)=>{
          if (e.key === 'Escape'){ H.close(); }
          else if (H.mode==='omnibox' && e.key === 'Enter'){
            const pick = H.suggestions && H.suggestions[H.selectedIndex|0];
            const v = (pick && pick.url) || input.value || '';
            if (v) H.post({type:'navigate', url:v});
            H.toggle(false);
          } else if (H.mode==='find' && e.key === 'Enter'){
            e.preventDefault();
            if (e.shiftKey) {
              H.jump(-1);
            } else if (e.altKey) {
              // Center on current match but keep overlay open for further navigation
              H.revealSelection();
            } else {
              // Default: center and close
              H.revealSelection();
              H.close();
            }
          } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp'){
            e.preventDefault(); e.stopPropagation();
            const n = (H.mode==='find'? H.matches.length : H.suggestions.length);
            if (n){
              H.selectedIndex = (H.selectedIndex||0) + (e.key==='ArrowDown'?1:-1);
              if (H.selectedIndex<0) H.selectedIndex = n-1; if (H.selectedIndex>=n) H.selectedIndex = 0;
              H.paintList();
              H.scrollListToSelection();
              if (H.mode==='find') { H.revealSelection(); H.setFaded(true); }
            }
          } else {
            if (H.mode==='find') H.setFaded(false);
          }
        });
        input.addEventListener('input', ()=> { H.onInput(); if (H.mode==='find') H.setFaded(false); });
        input.addEventListener('mousedown', ()=> H.setFaded(false));
        input.addEventListener('click', ()=> H.setFaded(false));
        H.el = root; H.input = input; H.list = list; H.selectedIndex = -1; H.paintList();
      },
      onInput(){
        const q = H.input.value || '';
        if (H.mode === 'omnibox'){
          clearTimeout(H.debounceTimer);
          H.debounceTimer = setTimeout(()=> H.post({type:'query', q, limit:10}), 120);
        } else {
          H.find(q);
        }
      },
      paintList(){
        const list = H.list; if (!list) return; list.textContent = '';
        if (H.mode==='omnibox'){
          H.suggestions.forEach((s, i)=>{
            const item = document.createElement('div');
            item.style.cssText = 'padding:8px 10px;display:flex;gap:10px;align-items:center;cursor:pointer;border-bottom:1px solid #2a2a2a;'+(i===H.selectedIndex?'background:#0a0a0a;':'');
            // Favicon
            const icon = document.createElement('img');
            icon.src = s.favicon || '';
            icon.width = 18; icon.height = 18; icon.loading = 'lazy';
            icon.style.cssText = 'flex:0 0 18px;width:18px;height:18px;border-radius:4px;opacity:.95;';
            icon.onerror = ()=>{ icon.style.display='none'; };
            // Text line: Domain | full path (one line, fade at end)
            let domain = '', path = '';
            try { const u = new URL(s.url, window.location.href); domain = u.hostname; path = (u.pathname||'') + (u.search||'') + (u.hash||''); } catch(_) { domain = s.url || ''; }
            const text = document.createElement('div');
            text.style.cssText = 'flex:1;min-width:0;display:flex;gap:8px;align-items:center;white-space:nowrap;overflow:hidden;';
            // Apply gradient fade using mask for WebKit
            text.style.webkitMaskImage = 'linear-gradient(90deg, black 85%, transparent 100%)';
            text.style.maskImage = 'linear-gradient(90deg, black 85%, transparent 100%)';
            const domainEl = document.createElement('span'); domainEl.textContent = domain; domainEl.style.cssText = 'color:#e6e6e6;opacity:.95;';
            const sep = document.createElement('span'); sep.textContent = ' | '; sep.style.cssText = 'color:#777;';
            const pathEl = document.createElement('span'); pathEl.textContent = path || '/'; pathEl.style.cssText = 'color:#9ad;';
            text.appendChild(domainEl); text.appendChild(sep); text.appendChild(pathEl);
            item.appendChild(icon); item.appendChild(text);
            item.addEventListener('mouseenter', ()=>{ H.selectedIndex = i; H.paintList(); H.scrollListToSelection(); });
            item.addEventListener('click',()=>{ H.post({type:'navigate', url:s.url}); H.toggle(false); });
            list.appendChild(item);
          });
        } else {
          const total = H.matches.length;
          const header = document.createElement('div');
          header.textContent = total ? total + ' matches' : 'No matches';
          header.style.cssText = 'padding:6px 10px;color:#bbb;font-size:12px;border-bottom:1px solid #2a2a2a;';
          list.appendChild(header);
          H.matches.forEach((m, i)=>{
            const item = document.createElement('div');
            item.style.cssText = 'padding:8px 10px;cursor:pointer;border-bottom:1px solid #2a2a2a;'+(i===H.selectedIndex?'background:#0a0a0a;':'');
            const ctx = document.createElement('div'); ctx.textContent = m.context || ''; ctx.style.cssText = 'color:#ddd;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
            item.appendChild(ctx);
            item.addEventListener('mouseenter', ()=>{ H.selectedIndex = i; H.paintList(); H.scrollListToSelection(); H.setFaded(true); });
            item.addEventListener('click',()=>{ H.selectedIndex=i; H.revealSelection(); H.paintList(); H.close(); });
            list.appendChild(item);
          });
        }
      },
      setSuggestions(arr){ H.suggestions = Array.isArray(arr)? arr: []; H.selectedIndex = -1; H.paintList(); },
      // FIND MODE IMPLEMENTATION
      clearHighlights(){
        try {
          H.highlightNodes.forEach(({span, text})=>{
            const p = span.parentNode; if (!p) return; p.replaceChild(text, span); p.normalize();
          });
        } catch(_){}
        H.highlightNodes = [];
        H.matches = [];
        H.selectedIndex = -1;
        H.activeIndex = -1;
        H.paintList();
      },
      find(q){
        H.clearHighlights();
        q = (q||'').trim(); if (!q) return;
        const body = document.body; if (!body) return;
        const root = document.getElementById('dumber-omnibox-root');
        const walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT, {
          acceptNode(node){
            if (!node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
            let el = node.parentElement; if (!el) return NodeFilter.FILTER_REJECT;
            if (root && root.contains(el)) return NodeFilter.FILTER_REJECT;
            const name = el.tagName; if (name==='SCRIPT'||name==='STYLE'||name==='NOSCRIPT') return NodeFilter.FILTER_REJECT;
            if (getComputedStyle(el).visibility==='hidden' || getComputedStyle(el).display==='none') return NodeFilter.FILTER_REJECT;
            return NodeFilter.FILTER_ACCEPT;
          }
        });
        const lc = q.toLowerCase();
        while (walker.nextNode()){
          const text = walker.currentNode;
          let s = text.nodeValue; let i = 0;
          while (s && (i = s.toLowerCase().indexOf(lc)) !== -1) {
            const before = document.createTextNode(s.slice(0, i));
            const match = document.createTextNode(s.slice(i, i+q.length));
            const afterVal = s.slice(i+q.length);
            const span = document.createElement('span'); span.className = 'dumber-find-highlight'; span.appendChild(match);
            const parent = text.parentNode; if (!parent) break;
            parent.insertBefore(before, text);
            parent.insertBefore(span, text);
            text.nodeValue = afterVal;
            H.highlightNodes.push({span, text: match});
            const leftCtx = before.nodeValue.slice(-30);
            let rightRaw = afterVal.slice(0, 30);
            const boundaryIdx = rightRaw.search(/[\.,;:\-]/);
            if (boundaryIdx !== -1) {
              rightRaw = rightRaw.slice(0, boundaryIdx+1);
            }
            const context = (leftCtx + match.nodeValue + rightRaw).replace(/\s+/g,' ').trim();
            H.matches.push({el: span, context});
            if (H.matches.length >= MAX_MATCHES) break;
            s = afterVal;
          }
          if (H.matches.length >= MAX_MATCHES) break;
        }
        H.selectedIndex = H.matches.length ? 0 : -1;
        H.paintList();
        H.revealSelection();
      },
      revealSelection(){
        const prev = H.matches[H.activeIndex|0];
        if (prev && prev.el && prev.el.classList) { prev.el.classList.remove('dumber-find-active'); }
        const m = H.matches[H.selectedIndex|0]; if (!m) return;
        if (m.el && m.el.classList) { m.el.classList.add('dumber-find-active'); }
        H.activeIndex = H.selectedIndex|0;
        try { m.el.scrollIntoView({block:'center', inline:'nearest'}); } catch(_){ m.el.scrollIntoView(); }
      },
      jump(delta){
        const n = H.matches.length; if (!n) return;
        H.selectedIndex = ((H.selectedIndex||0) + delta) % n; if (H.selectedIndex<0) H.selectedIndex = n-1;
        H.paintList(); H.revealSelection();
      }
    };

    // Keyboard hooks
    window.addEventListener('keydown', (e)=>{
      const mod = (e.ctrlKey||e.metaKey);
      if (mod && (e.key==='l' || e.key==='L')) { e.preventDefault(); H.open('omnibox'); }
      if (mod && (e.key==='f' || e.key==='F')) { e.preventDefault(); H.open('find'); }
    }, true);

    // Public API for native
    window.__dumber_setSuggestions = (arr)=> H.setSuggestions(arr);
    window.__dumber_toggle = ()=> H.toggle();
    window.__dumber_find_open = (q)=> H.open('find', q||'');
    window.__dumber_find_close = ()=> H.close();
    window.__dumber_find_query = (q)=> { if (H.mode!=='find') H.setMode('find'); if (!H.visible) H.toggle(true); H.input.value = q||''; H.find(q||''); };
  } catch (e) { /* no-op */ }
})();`
}
