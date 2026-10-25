import { useState, useCallback, useEffect, useRef } from "react";
import type { PrinterStatusKind } from "../components/PrinterStatusBar";
import { printerApi } from "../api/endpoints";
import { FILE_PRINTER } from "../constants";
import { useSSE } from "./useSSE";

// ── Types ──

export interface PrinterInfo {
  id: string;
  name: string;
}

export interface LabelStatus {
  model_name: string;
  media_type: string;
  media_width: number;
  media_length: number;
  status_type: string;
  phase_type: string;
  errors: string[];
}

export interface PrinterStatusState {
  /** Available printers from /api/printers */
  printers: PrinterInfo[];
  /** Resolved status kind for display */
  status: PrinterStatusKind;
  /** Optional detail string (phase_type, error message) */
  statusDetail: string | undefined;
  /** Raw label status from /api/status */
  labelStatus: LabelStatus | null;
  /** Whether the initial printer list is loading */
  loading: boolean;
  /** Error message from fetch failures */
  error: string | null;
  /** Whether a manual refresh is debounced */
  refreshDebounced: boolean;
  /** Manually trigger a full refresh (printers + status) */
  refresh: () => void;
  /** Change selected printer by id */
  selectPrinter: (id: string) => void;
  /** Re-fetch status for the current printer */
  fetchStatus: () => Promise<void>;
}

// ── Constants ──

const STORAGE_KEY = "selectedPrinter";
const PRINTER_NAME_KEY = "selectedPrinterName";
const REFRESH_DEBOUNCE = 2_000;

// ── Helpers ──

function detectLabelSizeFromStatus(status: LabelStatus): string | null {
  const { media_width: width, media_length: length, media_type } = status;
  const isContinuous = media_type === "Continuous length tape";

  if (isContinuous) {
    const map: Record<number, string> = {
      12: "12", 18: "18", 29: "29", 38: "38",
      50: "50", 54: "54", 62: "62", 102: "102", 104: "103",
    };
    return map[width] || null;
  }

  const map: Record<string, string> = {
    "17x54": "17x54", "17x87": "17x87", "23x23": "23x23",
    "29x42": "29x42", "29x90": "29x90", "38x90": "39x90",
    "39x48": "39x48", "52x29": "52x29", "54x29": "54x29",
    "60x87": "60x86", "62x29": "62x29", "62x100": "62x100",
    "102x51": "102x51", "102x153": "102x152", "104x164": "103x164",
  };
  return map[`${width}x${length}`] || null;
}

function resolveStatus(
  printerId: string,
  labelStatus: LabelStatus | null,
  error: string | null,
): { kind: PrinterStatusKind; detail?: string } {
  if (printerId === "file") return { kind: "file" };
  if (error) return { kind: "error", detail: error };
  if (labelStatus?.errors && labelStatus.errors.length > 0)
    return { kind: "error", detail: labelStatus.errors.join(", ") };
  if (labelStatus?.phase_type === "Waiting to receive") return { kind: "ready" };
  if (labelStatus) return { kind: "busy", detail: labelStatus.phase_type };
  return { kind: "offline" };
}

function loadSavedPrinter(): { id: string | null; name: string | null } {
  try {
    const savedId = localStorage.getItem(STORAGE_KEY);
    const savedName = localStorage.getItem(PRINTER_NAME_KEY);
    return {
      id: savedId ? JSON.parse(savedId) : null,
      name: savedName ? JSON.parse(savedName) : null,
    };
  } catch {
    return { id: null, name: null };
  }
}

function savePrinterToStorage(id: string, name?: string) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(id));
    if (name) localStorage.setItem(PRINTER_NAME_KEY, JSON.stringify(name));
  } catch { /* silent */ }
}

// ── Hook ──

export function usePrinterStatus(
  selectedPrinter: PrinterInfo,
  onSelectPrinter: (printer: PrinterInfo) => void,
  options?: {
    settingsMode?: "auto" | "manual";
    onLabelSizeChange?: (size: { id: string }) => void;
  },
): PrinterStatusState {
  const settingsMode = options?.settingsMode;
  const onLabelSizeChange = options?.onLabelSizeChange;

  const [printers, setPrinters] = useState<PrinterInfo[]>([]);
  const [labelStatus, setLabelStatus] = useState<LabelStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshDebounced, setRefreshDebounced] = useState(false);
  const hasInitialized = useRef(false);

  // ── Fetch printer list ──

  const fetchPrinterList = useCallback(async (): Promise<PrinterInfo[]> => {
    const data = await printerApi.list();
    return data.printers || [];
  }, []);

  const autoSelectPrinter = useCallback((list: PrinterInfo[]) => {
    const saved = loadSavedPrinter();

    if (saved.id === "file") { onSelectPrinter(FILE_PRINTER); return; }

    if (saved.id || saved.name) {
      const byId = list.find((p) => p.id === saved.id);
      if (byId) { onSelectPrinter(byId); return; }

      const byName = saved.name ? list.find((p) => p.name === saved.name) : null;
      if (byName) {
        onSelectPrinter(byName);
        savePrinterToStorage(byName.id, byName.name);
        return;
      }
    }

    if (list.length > 0) {
      onSelectPrinter(list[0]);
      savePrinterToStorage(list[0].id, list[0].name);
    } else {
      onSelectPrinter(FILE_PRINTER);
    }
  }, [onSelectPrinter]);

  const refreshPrinters = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const list = await fetchPrinterList();
      setPrinters(list);
      if (!hasInitialized.current) {
        hasInitialized.current = true;
        autoSelectPrinter(list);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  }, [fetchPrinterList, autoSelectPrinter]);

  // ── Fetch status ──

  const fetchStatus = useCallback(async () => {
    if (!selectedPrinter?.id || selectedPrinter.id === "file") {
      setLabelStatus(null);
      return;
    }
    try {
      const data = await printerApi.status({ printer: selectedPrinter.id });
      setError(null);
      setLabelStatus(data.status);

      if (settingsMode === "auto" && onLabelSizeChange) {
        const detected = detectLabelSizeFromStatus(data.status);
        if (detected) onLabelSizeChange({ id: detected });
      }
    } catch (err) {
      setLabelStatus(null);
      setError(err instanceof Error ? err.message : "Cannot reach printer");
    }
  }, [selectedPrinter?.id, settingsMode, onLabelSizeChange]);

  // ── Public actions ──

  const selectPrinter = useCallback((id: string) => {
    const printer = printers.find((p) => p.id === id) || FILE_PRINTER;
    onSelectPrinter(printer);
    savePrinterToStorage(printer.id, printer.name);
    setLabelStatus(null);
  }, [printers, onSelectPrinter]);

  const refresh = useCallback(() => {
    if (refreshDebounced) return;
    setRefreshDebounced(true);
    setError(null);
    hasInitialized.current = false;
    refreshPrinters().then(() => fetchStatus());
    setTimeout(() => setRefreshDebounced(false), REFRESH_DEBOUNCE);
  }, [refreshDebounced, refreshPrinters, fetchStatus]);

  // ── Effects ──

  // Initial fetch
  useEffect(() => { refreshPrinters(); }, [refreshPrinters]);

  // Fetch status when printer changes or loading finishes
  useEffect(() => {
    if (selectedPrinter?.id && !loading) fetchStatus();
  }, [selectedPrinter?.id, loading, fetchStatus]);

  // Visibility/focus refresh
  useEffect(() => {
    const onVisible = () => { if (document.visibilityState === "visible") refreshPrinters(); };
    const onFocus = () => refreshPrinters();
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", onFocus);
    return () => {
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", onFocus);
    };
  }, [refreshPrinters]);

  // SSE: listen for server-pushed printer list updates
  const selectedPrinterRef = useRef(selectedPrinter);
  selectedPrinterRef.current = selectedPrinter;

  useSSE({
    pollFallback: { printers: () => printerApi.list() },
    events: {
      printers: (data) => {
        const list = (data as { printers: PrinterInfo[] }).printers || [];
        setPrinters(list);

        if (!hasInitialized.current && list.length > 0) {
          hasInitialized.current = true;
          autoSelectPrinter(list);
          return;
        }

        // If selected printer disappeared, mark offline (don't switch to file)
        const current = selectedPrinterRef.current;
        if (current && current.id !== "file") {
          const stillExists = list.some((p) => p.id === current.id);
          if (!stillExists) {
            // Printer gone — clear status so resolveStatus returns "offline"
            setLabelStatus(null);
            setError(null);
          }
        }

        // If currently "offline" (non-file printer not in list), check if it came back
        // — possibly at a new USB address but same name
        if (current && current.id !== "file") {
          const backById = list.find((p) => p.id === current.id);
          const backByName = !backById
            ? list.find((p) => p.name === current.name)
            : null;
          if (backByName) {
            // Printer returned at new address
            onSelectPrinter(backByName);
            savePrinterToStorage(backByName.id, backByName.name);
          }
        }
      },
    },
  });

  // ── Resolve status ──

  const { kind, detail } = resolveStatus(selectedPrinter?.id, labelStatus, error);

  return {
    printers,
    status: kind,
    statusDetail: detail,
    labelStatus,
    loading,
    error,
    refreshDebounced,
    refresh,
    selectPrinter,
    fetchStatus,
  };
}

export { FILE_PRINTER, STORAGE_KEY, PRINTER_NAME_KEY, savePrinterToStorage };
