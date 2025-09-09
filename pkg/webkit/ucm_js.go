//go:build webkit_cgo

package webkit

// ucmOmniboxScript is injected at document-start via WebKit UserContentManager.
// It renders a minimal overlay with an input and a suggestions list, handles
// Ctrl+L to toggle, Enter to navigate, and posts search queries to native.
const ucmOmniboxScript = `(() => {
  try {
    if (window.__dumber_omnibox_loaded) return; // idempotent
    window.__dumber_omnibox_loaded = true;

    const H = {
      el: null,
      input: null,
      list: null,
      visible: false,
      suggestions: [],
      debounceTimer: 0,
      post(msg){ try { window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.dumber && window.webkit.messageHandlers.dumber.postMessage(JSON.stringify(msg)); } catch(_){} },
      render(){
        if (!H.el) H.mount();
        H.el.style.display = H.visible ? 'block' : 'none';
        if (H.visible) H.input.focus();
      },
      mount(){
        const root = document.createElement('div');
        root.id = 'dumber-omnibox-root';
        root.style.cssText = 'position:fixed;inset:0;z-index:2147483647;display:none;';
        const box = document.createElement('div');
        box.style.cssText = 'max-width:720px;margin:8vh auto;padding:8px 10px;background:#1b1b1b;color:#eee;border:1px solid #444;border-radius:8px;box-shadow:0 10px 30px rgba(0,0,0,.6);font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,\"Helvetica Neue\",Arial,sans-serif;';
        const input = document.createElement('input');
        input.type = 'text';
        input.placeholder = 'Type URL or search…';
        input.style.cssText = 'width:100%;padding:10px 12px;border-radius:6px;border:1px solid #555;background:#121212;color:#eee;font-size:16px;outline:none;';
        const list = document.createElement('div');
        list.style.cssText = 'margin-top:8px;max-height:50vh;overflow:auto;border-top:1px solid #333;';
        box.appendChild(input); box.appendChild(list); root.appendChild(box); document.documentElement.appendChild(root);
        input.addEventListener('keydown', (e)=>{
          if (e.key === 'Escape'){ H.toggle(false); }
          else if (e.key === 'Enter'){
            const pick = H.suggestions && H.suggestions[H.selectedIndex|0];
            const v = (pick && pick.url) || input.value || '';
            if (v) H.post({type:'navigate', url:v});
            H.toggle(false);
          } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp'){
            e.preventDefault();
            const n = H.suggestions.length;
            if (n){
              H.selectedIndex = (H.selectedIndex||0) + (e.key==='ArrowDown'?1:-1);
              if (H.selectedIndex<0) H.selectedIndex = n-1; if (H.selectedIndex>=n) H.selectedIndex = 0;
              H.paintList();
            }
          }
        });
        input.addEventListener('input', ()=>{
          clearTimeout(H.debounceTimer);
          const q = input.value;
          H.debounceTimer = setTimeout(()=> H.post({type:'query', q, limit:10}), 120);
        });
        H.el = root; H.input = input; H.list = list; H.selectedIndex = -1; H.paintList();
      },
      paintList(){
        const list = H.list; if (!list) return;
        list.textContent = '';
        H.suggestions.forEach((s, i)=>{
          const item = document.createElement('div');
          item.style.cssText = 'padding:8px 10px;display:flex;gap:10px;align-items:center;cursor:pointer;border-bottom:1px solid #2a2a2a;'+(i===H.selectedIndex?'background:#0a0a0a;':'');
          const url = document.createElement('div'); url.textContent = s.url || ''; url.style.cssText = 'flex:1;color:#9ad;word-break:break-all;';
          const title = document.createElement('div'); title.textContent = s.title || ''; title.style.cssText = 'flex:1;color:#ccc;opacity:.9;';
          item.appendChild(title); item.appendChild(url);
          item.addEventListener('click',()=>{ H.post({type:'navigate', url:s.url}); H.toggle(false); });
          list.appendChild(item);
        });
      },
      toggle(v){ H.visible = (typeof v==='boolean')? v : !H.visible; H.render(); },
      setSuggestions(arr){ H.suggestions = Array.isArray(arr)? arr: []; H.selectedIndex = -1; H.paintList(); }
    };
    window.addEventListener('keydown', (e)=>{
      if ((e.ctrlKey||e.metaKey) && (e.key==='l' || e.key==='L')) { e.preventDefault(); H.toggle(true); }
    }, true);
    // Allow native side to update suggestions or toggle
    window.__dumber_setSuggestions = (arr)=> H.setSuggestions(arr);
    window.__dumber_toggle = ()=> H.toggle();
  } catch (e) { /* no-op */ }
})();`
