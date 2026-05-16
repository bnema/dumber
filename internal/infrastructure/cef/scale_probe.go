package cef

const cefScaleProbeScript = `(function(){
  try {
    var vv = window.visualViewport;
    var probe = document.createElement('div');
    probe.style.cssText = 'position:fixed;left:0;top:0;width:100px;height:100px;visibility:hidden;pointer-events:none;z-index:-1';
    document.documentElement.appendChild(probe);
    var rect = probe.getBoundingClientRect();
    probe.remove();
    var data = {
      dpr: window.devicePixelRatio,
      inner: [window.innerWidth, window.innerHeight],
      outer: [window.outerWidth, window.outerHeight],
      client: [document.documentElement.clientWidth, document.documentElement.clientHeight],
      screen: [window.screen.width, window.screen.height],
      avail: [window.screen.availWidth, window.screen.availHeight],
      visualViewport: vv ? {
        width: vv.width,
        height: vv.height,
        scale: vv.scale,
        offsetLeft: vv.offsetLeft,
        offsetTop: vv.offsetTop,
        pageLeft: vv.pageLeft,
        pageTop: vv.pageTop
      } : null,
      css100Rect: {width: rect.width, height: rect.height, left: rect.left, top: rect.top},
      scroll: [window.scrollX, window.scrollY]
    };
    console.info('[SCALE-PROBE] ' + JSON.stringify(data));
  } catch (e) {
    console.warn('[SCALE-PROBE] error ' + (e && e.message ? e.message : String(e)));
  }
})();`
