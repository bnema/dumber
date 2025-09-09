// Wails service interfaces matching Go backend

export interface NavigationResult {
  url: string;
  title: string;
  success: boolean;
  error?: string;
  load_time: number;
  redirect_chain?: string[];
}

export interface HistoryEntry {
  id: number;
  url: string;
  title: string;
  visit_count: number;
  last_visited: string;
  created_at: string;
}

export interface SearchShortcut {
  description: string;
  url: string;
}

export interface Config {
  SearchShortcuts: Record<string, SearchShortcut>;
}

export interface BrowserService {
  Navigate(url: string): Promise<NavigationResult>;
  UpdatePageTitle(url: string, title: string): Promise<void>;
  GetRecentHistory(limit: number): Promise<HistoryEntry[]>;
  SearchHistory(query: string, limit: number): Promise<HistoryEntry[]>;
  DeleteHistoryEntry(id: number): Promise<void>;
  ClearHistory(): Promise<void>;
  GetHistoryStats(): Promise<Record<string, any>>;
  GetConfig(): Promise<Config>;
  UpdateConfig(config: Config): Promise<void>;
  GetSearchShortcuts(): Promise<Record<string, SearchShortcut>>;
  
  // Zoom functionality
  ZoomIn(url: string): Promise<number>;
  ZoomOut(url: string): Promise<number>;
  ResetZoom(url: string): Promise<number>;
  GetZoomLevel(url: string): Promise<number>;
  SetZoomLevel(url: string, zoomLevel: number): Promise<void>;
  
  // Navigation
  GetCurrentURL(): Promise<string>;
  CopyCurrentURL(url: string): Promise<void>;
  GoBack(): Promise<void>;
  GoForward(): Promise<void>;
}

export interface ParserService {
  ParseInput(input: string): Promise<any>;
}

export interface ConfigService {
  GetSearchShortcuts(): Promise<Record<string, SearchShortcut>>;
}

export interface WailsServices {
  BrowserService: BrowserService;
  ParserService: ParserService;
  ConfigService: ConfigService;
}

// Global Wails interface
declare global {
  interface Window {
    go?: {
      services?: WailsServices;
    };
  }
}