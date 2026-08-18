package webutil

import "fmt"

// BuildPageScrollByJS returns one immediate JavaScript scroll step for the given
// CSS-pixel delta. It resolves the initial target under the viewport center,
// walks toward scrollable ancestors that can move in the requested direction,
// and hands off to the document when a nested scroller reaches its boundary.
// Cross-origin frame contents remain best-effort because elementFromPoint can
// only return the accessible frame element from the top-level document.
//
// The app-level Vim Mode repeater owns held-key cadence; this helper performs
// no requestAnimationFrame loop of its own.
func BuildPageScrollByJS(dx, dy int) string {
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
  var beforeLeft=el.scrollLeft,beforeTop=el.scrollTop;
  try{
    if(dx!==0)el.scrollLeft=beforeLeft+dx;
    if(dy!==0)el.scrollTop=beforeTop+dy;
  }catch(_){
    return false;
  }
  return el.scrollLeft!==beforeLeft||el.scrollTop!==beforeTop;
}
try{
  var node=typeof doc.elementFromPoint==='function'?
    doc.elementFromPoint(window.innerWidth/2,window.innerHeight/2):null;
  while(node&&node!==doc.body&&node!==doc.documentElement){
    if(canScroll(node)&&scrollElement(node))return;
    node=node.parentElement;
  }
  var scroller=doc.scrollingElement||doc.documentElement;
  if(scroller&&canScroll(scroller)&&scrollElement(scroller))return;
  if(typeof window.scrollBy==='function'){
    window.scrollBy(dx,dy);
    return;
  }
  if(typeof window.scrollTo==='function'){
    window.scrollTo(window.scrollX+dx,window.scrollY+dy);
  }
}catch(e){}
})()`, dx, dy)
}
