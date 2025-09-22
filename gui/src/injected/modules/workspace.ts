/**
 * Workspace Controller Module
 *
 * Provides the initial scaffolding for Zellij-inspired pane management.
 * Currently tracks pane mode state, exposes configuration to the page,
 * and wires keyboard service shortcuts based on configuration defaults.
 */

import { keyboardService } from "$lib/keyboard";

const ACTIVATION_COMPONENT_ID = "workspace-pane-activation";
const ACTION_COMPONENT_ID = "workspace-pane-actions";

export interface WorkspaceConfigPayload {
  enable_zellij_controls?: boolean;
  pane_mode?: {
    activation_shortcut?: string;
    timeout_ms?: number;
    action_bindings?: Record<string, string>;
  };
  tabs?: {
    new_tab?: string;
    close_tab?: string;
    next_tab?: string;
    previous_tab?: string;
  };
  popups?: {
    placement?: string;
    open_in_new_pane?: boolean;
    follow_pane_context?: boolean;
  };
}

export interface WorkspacePaneModeConfig {
  activationShortcut: string;
  timeoutMs: number;
  actionBindings: Record<string, string>;
}

export interface WorkspaceTabsConfig {
  newTab: string;
  closeTab: string;
  nextTab: string;
  previousTab: string;
}

export interface WorkspacePopupConfig {
  placement: string;
  openInNewPane: boolean;
  followPaneContext: boolean;
}

export interface WorkspaceConfigNormalized {
  enableZellijControls: boolean;
  paneMode: WorkspacePaneModeConfig;
  tabs: WorkspaceTabsConfig;
  popups: WorkspacePopupConfig;
}

export interface WorkspaceRuntime {
  enterPaneMode: () => void;
  exitPaneMode: (reason?: string) => void;
  isPaneModeActive: () => boolean;
  getConfig: () => WorkspaceConfigNormalized;
  getInstanceId: () => string;
  isActiveInstance: () => boolean;
}

const DEFAULT_CONFIG: WorkspaceConfigNormalized = {
  enableZellijControls: true,
  paneMode: {
    activationShortcut: "cmdorctrl+p",
    timeoutMs: 3000,
    actionBindings: {
      arrowright: "split-right",
      arrowleft: "split-left",
      arrowup: "split-up",
      arrowdown: "split-down",
      r: "split-right",
      l: "split-left",
      u: "split-up",
      d: "split-down",
      x: "close-pane",
      enter: "confirm",
      escape: "cancel",
    },
  },
  tabs: {
    newTab: "cmdorctrl+t",
    closeTab: "cmdorctrl+w",
    nextTab: "cmdorctrl+tab",
    previousTab: "cmdorctrl+shift+tab",
  },
  popups: {
    placement: "right",
    openInNewPane: true,
    followPaneContext: true,
  },
};

function normalizeWorkspaceConfig(
  payload?: WorkspaceConfigPayload | null,
): WorkspaceConfigNormalized {
  const paneModePayload = payload?.pane_mode ?? {};
  const tabsPayload = payload?.tabs ?? {};
  const popupsPayload = payload?.popups ?? {};

  return {
    enableZellijControls:
      payload?.enable_zellij_controls ?? DEFAULT_CONFIG.enableZellijControls,
    paneMode: {
      activationShortcut:
        paneModePayload.activation_shortcut ??
        DEFAULT_CONFIG.paneMode.activationShortcut,
      timeoutMs:
        paneModePayload.timeout_ms ?? DEFAULT_CONFIG.paneMode.timeoutMs,
      actionBindings: {
        ...DEFAULT_CONFIG.paneMode.actionBindings,
        ...(paneModePayload.action_bindings ?? {}),
      },
    },
    tabs: {
      newTab: tabsPayload.new_tab ?? DEFAULT_CONFIG.tabs.newTab,
      closeTab: tabsPayload.close_tab ?? DEFAULT_CONFIG.tabs.closeTab,
      nextTab: tabsPayload.next_tab ?? DEFAULT_CONFIG.tabs.nextTab,
      previousTab: tabsPayload.previous_tab ?? DEFAULT_CONFIG.tabs.previousTab,
    },
    popups: {
      placement: popupsPayload.placement ?? DEFAULT_CONFIG.popups.placement,
      openInNewPane:
        popupsPayload.open_in_new_pane ?? DEFAULT_CONFIG.popups.openInNewPane,
      followPaneContext:
        popupsPayload.follow_pane_context ??
        DEFAULT_CONFIG.popups.followPaneContext,
    },
  };
}

class WorkspaceController implements WorkspaceRuntime {
  private config: WorkspaceConfigNormalized = DEFAULT_CONFIG;
  private paneModeActive = false;
  private paneModeTimer: ReturnType<typeof setTimeout> | null = null;
  private instanceId: string;
  private lastActiveCheckTime = 0;

  constructor() {
    // Generate unique instance ID for this webview
    this.instanceId = `workspace-${Date.now()}-${Math.random().toString(36).substring(2)}`;
    console.log(
      `[workspace] Created WorkspaceController instance: ${this.instanceId}`,
    );
  }

  getInstanceId(): string {
    return this.instanceId;
  }

  isActiveInstance(): boolean {
    // Cache active check for 50ms to avoid excessive checks
    const now = Date.now();
    if (now - this.lastActiveCheckTime < 50) {
      return true; // Assume active within debounce window
    }
    this.lastActiveCheckTime = now;

    // Check if this webview has focus by attempting to query document.hasFocus()
    // In a multi-pane environment, only the focused webview should handle pane mode
    try {
      const hasFocus = document.hasFocus();
      const isVisible = document.visibilityState === "visible";
      const isActive = hasFocus && isVisible;

      if (!isActive) {
        console.log(
          `[workspace] Instance ${this.instanceId} not active: focus=${hasFocus} visible=${isVisible}`,
        );
      }

      return isActive;
    } catch (error) {
      console.warn(
        "[workspace] Failed to check active state, assuming active:",
        error,
      );
      return true; // Fallback to active if check fails
    }
  }

  configure(payload?: WorkspaceConfigPayload | null): void {
    const normalized = normalizeWorkspaceConfig(payload);
    this.unregisterShortcuts();
    this.config = normalized;

    if (!normalized.enableZellijControls) {
      console.log("[workspace] Zellij controls disabled via config");
      window.__dumber_workspace = this.exportRuntime();
      return;
    }

    this.registerShortcuts();
    window.__dumber_workspace = this.exportRuntime();
    console.log(
      "[workspace] Zellij controls enabled with config:",
      this.config,
    );
  }

  private bridge(message: Record<string, unknown>): void {
    const bridge = window.webkit?.messageHandlers?.dumber;
    if (!bridge || typeof bridge.postMessage !== "function") {
      return;
    }

    try {
      bridge.postMessage(
        JSON.stringify({
          type: "workspace",
          ...message,
        }),
      );
    } catch (error) {
      console.error("[workspace] Failed to post workspace message", error);
    }
  }

  enterPaneMode(): void {
    if (!this.config.enableZellijControls) return;

    // Only allow the active instance to enter pane mode
    if (!this.isActiveInstance()) {
      console.log(
        `[workspace] Instance ${this.instanceId} ignoring pane mode - not active instance`,
      );
      return;
    }

    console.log(`[workspace] Instance ${this.instanceId} entering pane mode`);
    this.paneModeActive = true;
    this.restartPaneModeTimer();
    this.emitWorkspaceEvent("pane-mode-entered", {
      action: "enter",
      config: this.config,
      instanceId: this.instanceId,
    });
    this.bridge({ event: "pane-mode-entered", instanceId: this.instanceId });
    this.showToast("Pane mode: awaiting direction");
  }

  exitPaneMode(reason: string = "exit"): void {
    if (!this.paneModeActive) return;

    this.paneModeActive = false;
    if (this.paneModeTimer) {
      clearTimeout(this.paneModeTimer);
      this.paneModeTimer = null;
    }
    this.emitWorkspaceEvent("pane-mode-exited", { action: reason });
    this.bridge({ event: "pane-mode-exited", reason });
  }

  isPaneModeActive(): boolean {
    return this.paneModeActive;
  }

  getConfig(): WorkspaceConfigNormalized {
    return this.config;
  }

  private registerShortcuts(): void {
    keyboardService.registerShortcuts(ACTIVATION_COMPONENT_ID, [
      {
        key: this.config.paneMode.activationShortcut,
        handler: () => this.enterPaneMode(),
        description: "Enter pane mode",
        preventDefault: true,
        stopPropagation: true,
        whenFocused: false,
      },
    ]);

    const paneShortcuts = Object.entries(
      this.config.paneMode.actionBindings,
    ).map(([key, action]) => ({
      key,
      handler: () => this.handlePaneAction(action, key),
      description: `Pane action: ${action}`,
      preventDefault: true,
      stopPropagation: true,
      whenFocused: false,
    }));

    keyboardService.registerShortcuts(ACTION_COMPONENT_ID, paneShortcuts);
  }

  private unregisterShortcuts(): void {
    keyboardService.unregisterShortcuts(ACTIVATION_COMPONENT_ID);
    keyboardService.unregisterShortcuts(ACTION_COMPONENT_ID);
  }

  private handlePaneAction(action: string, key?: string): void {
    if (!this.paneModeActive) {
      return;
    }

    switch (action) {
      case "split-right":
        this.emitWorkspaceEvent("pane-split", {
          direction: "right",
          config: this.config,
        });
        this.bridge({ event: "pane-split", direction: "right" });
        this.showToast("Split pane to the right");
        this.exitPaneMode("split-right");
        break;
      case "split-left":
        this.emitWorkspaceEvent("pane-split", {
          direction: "left",
          config: this.config,
        });
        this.bridge({ event: "pane-split", direction: "left" });
        this.showToast("Split pane to the left");
        this.exitPaneMode("split-left");
        break;
      case "split-up":
        this.emitWorkspaceEvent("pane-split", {
          direction: "up",
          config: this.config,
        });
        this.bridge({ event: "pane-split", direction: "up" });
        this.showToast("Split pane upwards");
        this.exitPaneMode("split-up");
        break;
      case "split-down":
        this.emitWorkspaceEvent("pane-split", {
          direction: "down",
          config: this.config,
        });
        this.bridge({ event: "pane-split", direction: "down" });
        this.showToast("Split pane downwards");
        this.exitPaneMode("split-down");
        break;
      case "close-pane":
        this.emitWorkspaceEvent("pane-closed", { key, config: this.config });
        this.bridge({ event: "pane-close", action });
        this.showToast("Closed pane");
        this.exitPaneMode("close-pane");
        break;
      case "confirm":
        this.bridge({ event: "pane-confirmed" });
        this.showToast("Pane mode confirmed");
        this.exitPaneMode("confirm");
        break;
      case "cancel":
        this.bridge({ event: "pane-cancelled" });
        this.showToast("Pane mode cancelled");
        this.exitPaneMode("cancel");
        break;
      default:
        console.warn("[workspace] Unhandled pane action:", action);
    }
  }

  private restartPaneModeTimer(): void {
    if (this.paneModeTimer) {
      clearTimeout(this.paneModeTimer);
    }

    if (this.config.paneMode.timeoutMs > 0) {
      this.paneModeTimer = setTimeout(() => {
        this.showToast("Pane mode timed out");
        this.exitPaneMode("timeout");
      }, this.config.paneMode.timeoutMs);
    }
  }

  private showToast(message: string): void {
    if (typeof window.__dumber_showToast === "function") {
      window.__dumber_showToast(message, 2000, "info");
    } else {
      console.log("[workspace toast]", message);
    }
  }

  private emitWorkspaceEvent(
    event: string,
    detail: Record<string, unknown>,
  ): void {
    document.dispatchEvent(
      new CustomEvent(`dumber:workspace-${event}`, { detail }),
    );
  }

  private exportRuntime(): WorkspaceRuntime {
    return {
      enterPaneMode: () => this.enterPaneMode(),
      exitPaneMode: (reason?: string) =>
        this.exitPaneMode(reason ?? "external"),
      isPaneModeActive: () => this.isPaneModeActive(),
      getConfig: () => this.getConfig(),
      getInstanceId: () => this.getInstanceId(),
      isActiveInstance: () => this.isActiveInstance(),
    };
  }
}

const workspaceController = new WorkspaceController();
let listenersAttached = false;

export function initializeWorkspace(
  payload?: WorkspaceConfigPayload | null,
): void {
  if (!listenersAttached) {
    document.addEventListener("dumber:workspace-config", (event: Event) => {
      const detail = (event as CustomEvent<WorkspaceConfigPayload>).detail;
      workspaceController.configure(detail);
    });
    listenersAttached = true;
  }

  const bootstrapPayload = payload ?? window.__dumber_workspace_config ?? null;
  workspaceController.configure(bootstrapPayload);
}

export { workspaceController };
