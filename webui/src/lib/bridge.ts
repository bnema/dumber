/**
 * WebKit bridge utilities for communicating with the Go backend.
 *
 * IMPORTANT: Svelte 5 uses Proxy objects for reactive state ($state, $derived).
 * WebKit's messageHandlers.postMessage() cannot serialize Proxy objects - it
 * fails silently, dropping the message entirely. This module provides safe
 * serialization that converts Proxies to plain objects before sending.
 */

export interface BridgeMessage {
  type: string;
  webviewId: number;
  payload: Record<string, unknown>;
}

interface WebKitBridge {
  postMessage: (msg: unknown) => void;
}

/**
 * Get the WebKit message handler bridge, if available.
 */
export function getWebKitBridge(): WebKitBridge | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const bridge = (window as any).webkit?.messageHandlers?.dumber;
  return bridge && typeof bridge.postMessage === "function" ? bridge : null;
}

/**
 * Get the current webview ID assigned by the Go backend.
 */
export function getWebViewId(): number {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (window as any).__dumber_webview_id || 0;
}

/**
 * Deep-clone an object, converting all Svelte 5 Proxy objects to plain objects.
 * Uses JSON serialization which strips Proxy wrappers.
 *
 * @throws Error if the object contains non-serializable values (functions, circular refs, etc.)
 */
export function toPlainObject<T>(obj: T): T {
  try {
    return JSON.parse(JSON.stringify(obj));
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to serialize object for bridge: ${message}`);
  }
}

/**
 * Post a message to the Go backend via WebKit bridge.
 *
 * Automatically:
 * - Converts Svelte 5 Proxy objects to plain objects
 * - Adds webviewId if not provided
 * - Logs errors if serialization fails
 *
 * @throws Error if bridge is not available or serialization fails
 */
export function postMessage(msg: BridgeMessage): void {
  console.log("[bridge] postMessage called with:", msg);

  const bridge = getWebKitBridge();
  if (!bridge) {
    throw new Error("WebKit bridge not available");
  }

  // Ensure webviewId is set
  if (!msg.webviewId) {
    msg.webviewId = getWebViewId();
  }

  // Convert to plain object to strip Svelte 5 Proxies
  // This is critical - WebKit silently drops messages with Proxy objects
  let plainMsg: BridgeMessage;
  try {
    plainMsg = toPlainObject(msg);
    console.log("[bridge] Serialized message (Proxy stripped):", plainMsg);
  } catch (err) {
    console.error("[bridge] Failed to serialize message:", err, msg);
    throw err;
  }

  console.log("[bridge] Sending to WebKit bridge...");
  bridge.postMessage(plainMsg);
  console.log("[bridge] Message sent successfully");
}

/**
 * Post a message and set up success/error callbacks.
 *
 * @param type - Message type (e.g., "get_keybindings", "set_keybinding")
 * @param payload - Message payload (will be serialized to strip Proxies)
 * @param successCallback - Window function name to call on success
 * @param errorCallback - Window function name to call on error
 * @param onSuccess - Handler for success response
 * @param onError - Handler for error response
 */
export function postMessageWithCallbacks<TResponse, TPayload extends Record<string, unknown>>(
  type: string,
  payload: TPayload,
  successCallback: string,
  errorCallback: string,
  onSuccess: (response: TResponse) => void,
  onError: (error: string) => void,
): void {
  // Set up callbacks on window
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any)[successCallback] = (response: TResponse) => {
    onSuccess(response);
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any)[errorCallback] = (msg: string) => {
    onError(typeof msg === "string" ? msg : "Unknown error");
  };

  try {
    postMessage({
      type,
      webviewId: getWebViewId(),
      payload,
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    onError(message);
  }
}
