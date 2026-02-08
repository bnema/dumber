import "../styles/app.css";

const activeStreams: MediaStream[] = [];

function now(): string {
  return new Date().toLocaleTimeString();
}

function appendLog(output: HTMLPreElement, message: string): void {
  output.textContent = `${output.textContent}[${now()}] ${message}\n`;
  output.scrollTop = output.scrollHeight;
}

function setStatus(status: HTMLElement, value: string): void {
  status.textContent = value;
}

function stopAllStreams(video: HTMLVideoElement, output: HTMLPreElement, status: HTMLElement): void {
  for (const stream of activeStreams.splice(0)) {
    for (const track of stream.getTracks()) {
      track.stop();
    }
  }

  video.pause();
  video.srcObject = null;
  setStatus(status, "idle");
  appendLog(output, "Stopped all active tracks");
}

async function probeDevices(output: HTMLPreElement): Promise<void> {
  if (!("mediaDevices" in navigator) || typeof navigator.mediaDevices.enumerateDevices !== "function") {
    appendLog(output, "enumerateDevices is unavailable");
    return;
  }

  try {
    const devices = await navigator.mediaDevices.enumerateDevices();
    const summary = devices
      .map((device) => `${device.kind} ${device.label || "(label hidden until permission)"}`)
      .join(" | ");

    appendLog(output, `Devices: ${summary || "none"}`);
  } catch (err) {
    appendLog(output, `enumerateDevices failed: ${String(err)}`);
    appendLog(output, "Devices: none");
  }
}

async function requestUserMedia(video: HTMLVideoElement, output: HTMLPreElement, status: HTMLElement): Promise<void> {
  if (!("mediaDevices" in navigator) || typeof navigator.mediaDevices.getUserMedia !== "function") {
    appendLog(output, "getUserMedia is unavailable");
    return;
  }

  try {
    setStatus(status, "requesting camera + microphone");
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
    activeStreams.push(stream);
    video.srcObject = stream;
    await video.play();
    setStatus(status, "camera + microphone granted");
    appendLog(output, "camera + microphone permission granted");
    await probeDevices(output);
  } catch (err) {
    setStatus(status, "camera + microphone blocked");
    appendLog(output, `camera + microphone request failed: ${String(err)}`);
  }
}

async function requestDisplayMedia(video: HTMLVideoElement, output: HTMLPreElement, status: HTMLElement): Promise<void> {
  if (!("mediaDevices" in navigator) || typeof navigator.mediaDevices.getDisplayMedia !== "function") {
    appendLog(output, "getDisplayMedia is unavailable");
    return;
  }

  try {
    setStatus(status, "requesting screen capture");
    const stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: true });
    activeStreams.push(stream);
    video.srcObject = stream;
    await video.play();
    setStatus(status, "screen capture granted");
    appendLog(output, "screen capture permission granted");
  } catch (err) {
    setStatus(status, "screen capture blocked");
    appendLog(output, `screen capture request failed: ${String(err)}`);
  }
}

function mount(): void {
  document.title = "WebRTC Diagnostics";
  document.body.innerHTML = `
    <main class="page-shell" style="padding: 24px; max-width: 920px; margin: 0 auto; display: grid; gap: 16px;">
      <header style="display: grid; gap: 8px;">
        <h1 style="margin: 0; font-size: 1.8rem;">WebRTC Diagnostics</h1>
        <p style="margin: 0; opacity: 0.85;">Use this page to validate camera, microphone, and screen permission behavior.</p>
      </header>

      <section style="display: flex; flex-wrap: wrap; gap: 10px;">
        <button id="btn-av" type="button">Request camera + microphone</button>
        <button id="btn-screen" type="button">Request screen capture</button>
        <button id="btn-stop" type="button">Stop all streams</button>
      </section>

      <section style="display: grid; gap: 8px;">
        <strong>Status: <span id="status">idle</span></strong>
        <video id="preview" autoplay playsinline muted style="width: 100%; max-height: 320px; border-radius: 10px; background: #0c1522;"></video>
      </section>

      <section style="display: grid; gap: 8px;">
        <strong>Event Log</strong>
        <pre id="output" style="margin: 0; padding: 12px; border-radius: 10px; background: #0d1727; color: #dbe8f4; min-height: 180px; max-height: 280px; overflow: auto;">[${now()}] Ready\n</pre>
      </section>
    </main>
  `;

  const status = document.getElementById("status");
  const preview = document.getElementById("preview");
  const output = document.getElementById("output");
  const btnAV = document.getElementById("btn-av");
  const btnScreen = document.getElementById("btn-screen");
  const btnStop = document.getElementById("btn-stop");

  if (!(status instanceof HTMLElement)) {
    return;
  }
  if (!(preview instanceof HTMLVideoElement)) {
    return;
  }
  if (!(output instanceof HTMLPreElement)) {
    return;
  }
  if (!(btnAV instanceof HTMLButtonElement)) {
    return;
  }
  if (!(btnScreen instanceof HTMLButtonElement)) {
    return;
  }
  if (!(btnStop instanceof HTMLButtonElement)) {
    return;
  }

  btnAV.addEventListener("click", () => {
    requestUserMedia(preview, output, status).catch((err: unknown) => {
      appendLog(output, `unexpected user media error: ${String(err)}`);
    });
  });

  btnScreen.addEventListener("click", () => {
    requestDisplayMedia(preview, output, status).catch((err: unknown) => {
      appendLog(output, `unexpected display media error: ${String(err)}`);
    });
  });

  btnStop.addEventListener("click", () => {
    stopAllStreams(preview, output, status);
  });

  window.addEventListener("beforeunload", () => {
    stopAllStreams(preview, output, status);
  });
}

mount();
