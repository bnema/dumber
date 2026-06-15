package webutil

import "fmt"

// BuildScrollByJS returns a JavaScript string that scrolls a web page by the
// given CSS-pixel delta.
//
// Semantics (frontend scroll-target resolution):
//  1. Start from document.activeElement.
//  2. Walk up the DOM tree to find the nearest scrollable ancestor that can
//     still scroll in the requested direction.
//  3. Fall back to document.scrollingElement (or documentElement).
//  4. Fall back to window scrolling.
//
// The helper prefers native smooth scrolling when available, while keeping the
// target-resolution semantics stable. Older engines fall back to direct
// scrollLeft/scrollTop assignment.
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
function scrollElement(el){
  if(!el)return false;
  try{
    if(typeof el.scrollBy==='function'){
      el.scrollBy({left:dx,top:dy,behavior:'smooth'});
      return true;
    }
    var beforeLeft=el.scrollLeft,beforeTop=el.scrollTop;
    if(dx!==0)el.scrollLeft=beforeLeft+dx;
    if(dy!==0)el.scrollTop=beforeTop+dy;
    return el.scrollLeft!==beforeLeft||el.scrollTop!==beforeTop;
  }catch(_){
    return false;
  }
}
try{
  var node=doc.activeElement;
  while(node&&node!==doc.body&&node!==doc.documentElement){
    if(canScroll(node)&&scrollElement(node))return;
    node=node.parentElement;
  }
  var scroller=doc.scrollingElement||doc.documentElement;
  if(scroller&&canScroll(scroller)&&scrollElement(scroller))return;
  if(typeof window.scrollBy==='function'){
    window.scrollBy({left:dx,top:dy,behavior:'smooth'});
    return;
  }
  if(typeof window.scrollTo==='function'){
    window.scrollTo(window.scrollX+dx,window.scrollY+dy);
  }
}catch(e){}
})()`, dx, dy)
}
