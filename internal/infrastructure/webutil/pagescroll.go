package webutil

import "fmt"

// BuildScrollByJS returns a JavaScript string that scrolls a web page by the
// given CSS-pixel delta.
//
// THIS IS A FALLBACK IMPLEMENTATION. The primary page-scroll abstraction is
// port.PageScrollable.ScrollPage. Engines should attempt native scrolling
// first and call this function only when no native mechanism exists for the
// requested command.
//
// Semantics (frontend scroll-target resolution):
//  1. Start from document.activeElement.
//  2. Walk up the DOM tree to find the nearest scrollable ancestor that can
//     still scroll in the requested direction.
//  3. Fall back to document.scrollingElement (or documentElement).
//  4. Fall back to window scrolling.
//
// Repeated Page mode actions can arrive quickly while a key is held. Instead of
// asking the engine to start a brand new native smooth-scroll animation for each
// keystroke, the helper coalesces deltas into a tiny requestAnimationFrame loop.
// That keeps hold-to-scroll responsive without stacked, competing animations.
func BuildScrollByJS(dx, dy int) string {
	return fmt.Sprintf(`(function(){
var dx=%d,dy=%d,doc=document;
function hasScrollableOverflow(value){
  return value==='auto'||value==='scroll'||value==='overlay';
}
function canScroll(el){
  if(!el)return false;
  var style=window.getComputedStyle(el);
  if(dy!==0){
    var overflowY=style.overflowY||style.overflow;
    var maxTop=el.scrollHeight-el.clientHeight;
    if(hasScrollableOverflow(overflowY)){
      if((dy<0&&el.scrollTop>0)||(dy>0&&el.scrollTop<maxTop))return true;
    }
  }
  if(dx!==0){
    var overflowX=style.overflowX||style.overflow;
    var maxLeft=el.scrollWidth-el.clientWidth;
    if(hasScrollableOverflow(overflowX)){
      if((dx<0&&el.scrollLeft>0)||(dx>0&&el.scrollLeft<maxLeft))return true;
    }
  }
  return false;
}
function resolveScrollTarget(){
  var node=doc.activeElement;
  while(node&&node!==doc.body&&node!==doc.documentElement){
    if(canScroll(node))return {kind:'element',target:node};
    node=node.parentElement;
  }
  var scroller=doc.scrollingElement||doc.documentElement;
  if(scroller&&canScroll(scroller))return {kind:'element',target:scroller};
  if(typeof window.scrollBy==='function'||typeof window.scrollTo==='function'){
    return {kind:'window',target:window};
  }
  return null;
}
function nextStep(pending){
  if(pending===0)return 0;
  if(Math.abs(pending)<=1)return pending;
  var scaled=pending*0.35;
  if(pending>0)return Math.max(1, Math.round(scaled));
  return Math.min(-1, Math.round(scaled));
}
function applyElementStep(el, stepX, stepY){
  var beforeLeft=el.scrollLeft,beforeTop=el.scrollTop;
  if(stepX!==0)el.scrollLeft=beforeLeft+stepX;
  if(stepY!==0)el.scrollTop=beforeTop+stepY;
  return {movedX:el.scrollLeft-beforeLeft,movedY:el.scrollTop-beforeTop};
}
function applyWindowStep(stepX, stepY){
  var beforeX=window.scrollX||window.pageXOffset||0;
  var beforeY=window.scrollY||window.pageYOffset||0;
  if(typeof window.scrollBy==='function'){
    window.scrollBy(stepX,stepY);
  }else if(typeof window.scrollTo==='function'){
    window.scrollTo(beforeX+stepX,beforeY+stepY);
  }
  var afterX=window.scrollX||window.pageXOffset||0;
  var afterY=window.scrollY||window.pageYOffset||0;
  return {movedX:afterX-beforeX,movedY:afterY-beforeY};
}
function schedule(state){
  if(state.raf) return;
  state.raf=window.requestAnimationFrame(function tick(){
    state.raf=0;
    if(!state.target||state.pendingX===0&&state.pendingY===0)return;
    var stepX=nextStep(state.pendingX);
    var stepY=nextStep(state.pendingY);
    var moved;
    try{
      moved=state.kind==='window'
        ? applyWindowStep(stepX,stepY)
        : applyElementStep(state.target,stepX,stepY);
    }catch(_){
      state.pendingX=0;
      state.pendingY=0;
      return;
    }
    state.pendingX-=moved.movedX;
    state.pendingY-=moved.movedY;
    if(moved.movedX===0&&moved.movedY===0){
      state.pendingX=0;
      state.pendingY=0;
      return;
    }
    if(state.pendingX!==0||state.pendingY!==0)schedule(state);
  });
}
try{
  var resolved=resolveScrollTarget();
  if(!resolved)return;
  var state=window.__dumberPageModeScrollState;
  if(!state){
    state=window.__dumberPageModeScrollState={target:null,kind:'',pendingX:0,pendingY:0,raf:0};
  }
  if(state.target!==resolved.target||state.kind!==resolved.kind){
    state.target=resolved.target;
    state.kind=resolved.kind;
    state.pendingX=0;
    state.pendingY=0;
  }
  state.pendingX+=dx;
  state.pendingY+=dy;
  schedule(state);
}catch(e){}
})()`, dx, dy)
}
