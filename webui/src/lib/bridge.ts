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
  const plainMsg = toPlainObject(msg);
  bridge.postMessage(plainMsg);
}

/**
 * Internal callback registry using Map for tracking pending requests.
 * Callbacks must also be exposed on window for Go backend to call,
 * but we track them here for proper cleanup and to prevent leaks.
 */
const pendingCallbacks = new Map<string, {
  cleanup: () => void;
  timeoutId: ReturnType<typeof setTimeout>;
}>();

let callbackCounter = 0;
const CALLBACK_TIMEOUT_MS = 30000;

/**
 * Post a message and set up success/error callbacks with automatic cleanup.
 * Uses unique IDs to prevent naming conflicts between concurrent requests.
 *
 * @param type - Message type (e.g., "get_keybindings", "set_keybinding")
 * @param payload - Message payload (will be serialized to strip Proxies)
 * @param successCallback - Base name for success callback (will be made unique)
 * @param errorCallback - Base name for error callback (will be made unique)
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
  const id = ++callbackCounter;
  const successKey = `${successCallback}_${id}`;
  const errorKey = `${errorCallback}_${id}`;

  // Cleanup removes from both window and our tracking Map
  const cleanup = () => {
    const entry = pendingCallbacks.get(successKey);
    if (entry) {
      clearTimeout(entry.timeoutId);
      pendingCallbacks.delete(successKey);
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (window as any)[successKey];
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (window as any)[errorKey];
  };

  // Timeout prevents memory leaks if backend never responds
  const timeoutId = setTimeout(() => {
    cleanup();
    onError("Request timed out");
  }, CALLBACK_TIMEOUT_MS);

  // Track in our Map for proper lifecycle management
  pendingCallbacks.set(successKey, { cleanup, timeoutId });

  // Expose on window for Go backend (it calls window[callbackName](response))
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any)[successKey] = (response: TResponse) => {
    cleanup();
    onSuccess(response);
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any)[errorKey] = (msg: unknown) => {
    cleanup();
    onError(typeof msg === "string" ? msg : "Unknown error");
  };

  try {
    postMessage({
      type,
      webviewId: getWebViewId(),
      payload: {
        ...payload,
        successCallback: successKey,
        errorCallback: errorKey,
      },
    });
  } catch (err) {
    cleanup();
    onError(err instanceof Error ? err.message : String(err));
  }
}
