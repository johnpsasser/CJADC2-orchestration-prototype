import { useCallback, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { create } from 'zustand';
import { api } from '../api/client';
import type { CorrelatedTrack, SortConfig } from '../types';

// How long before a track is considered stale (30 seconds)
// This ensures tracks are purged shortly after sensor config changes
const STALE_TRACK_AGE_MS = 30 * 1000;
// How often to check for stale tracks (10 seconds)
const STALE_CHECK_INTERVAL_MS = 10 * 1000;

// Wrapper to track when each item was first seen in the UI
// This allows stable row positioning - new tracks at top, existing tracks don't move
interface TrackedItem {
  track: CorrelatedTrack;
  insertedAt: number; // Unix timestamp when first added to UI
}

// Zustand store for track state
interface TrackStore {
  tracks: Map<string, TrackedItem>;
  selectedTrackId: string | null;
  sortConfig: SortConfig;
  filter: {
    classification: string | null;
    threatLevel: string | null;
    type: string | null;
    searchQuery: string;
  };
  setTracks: (tracks: CorrelatedTrack[]) => void;
  updateTrack: (track: CorrelatedTrack) => void;
  deleteTrack: (trackId: string) => void;
  selectTrack: (trackId: string | null) => void;
  setSortConfig: (config: SortConfig) => void;
  setFilter: (filter: Partial<TrackStore['filter']>) => void;
  clearFilter: () => void;
  clearStaleTracks: (maxAgeMs: number) => void;
  clearAllTracks: () => void;
}

export const useTrackStore = create<TrackStore>((set) => ({
  tracks: new Map(),
  selectedTrackId: null,
  sortConfig: { key: 'insertedAt', direction: 'desc' },
  filter: {
    classification: null,
    threatLevel: null,
    type: null,
    searchQuery: '',
  },
  setTracks: (tracks) =>
    set((state) => {
      const now = Date.now();
      const newTracks = new Map<string, TrackedItem>();
      tracks.forEach((t) => {
        // Preserve insertedAt for existing tracks, set current time for new ones
        const existing = state.tracks.get(t.track_id);
        newTracks.set(t.track_id, {
          track: t,
          insertedAt: existing?.insertedAt ?? now,
        });
      });
      return { tracks: newTracks };
    }),
  updateTrack: (track) =>
    set((state) => {
      const newTracks = new Map(state.tracks);
      const existing = state.tracks.get(track.track_id);
      // Preserve insertedAt for existing tracks, new tracks get current time
      newTracks.set(track.track_id, {
        track,
        insertedAt: existing?.insertedAt ?? Date.now(),
      });
      return { tracks: newTracks };
    }),
  deleteTrack: (trackId) =>
    set((state) => {
      const newTracks = new Map(state.tracks);
      newTracks.delete(trackId);
      return {
        tracks: newTracks,
        selectedTrackId:
          state.selectedTrackId === trackId ? null : state.selectedTrackId,
      };
    }),
  selectTrack: (trackId) => set({ selectedTrackId: trackId }),
  setSortConfig: (config) => set({ sortConfig: config }),
  setFilter: (filter) =>
    set((state) => ({
      filter: { ...state.filter, ...filter },
    })),
  clearFilter: () =>
    set({
      filter: {
        classification: null,
        threatLevel: null,
        type: null,
        searchQuery: '',
      },
    }),
  clearStaleTracks: (maxAgeMs: number) =>
    set((state) => {
      const now = Date.now();
      const newTracks = new Map<string, TrackedItem>();
      state.tracks.forEach((item, id) => {
        const lastUpdated = item.track.last_updated || item.track.window_end;
        if (lastUpdated) {
          const trackTime = new Date(lastUpdated).getTime();
          if (now - trackTime <= maxAgeMs) {
            newTracks.set(id, item);
          }
        }
      });
      return {
        tracks: newTracks,
        selectedTrackId: newTracks.has(state.selectedTrackId ?? '')
          ? state.selectedTrackId
          : null,
      };
    }),
  clearAllTracks: () =>
    set({
      tracks: new Map(),
      selectedTrackId: null,
    }),
}));

// Query key for tracks
const TRACKS_QUERY_KEY = ['tracks'];

// Hook for fetching and managing tracks
export function useTracks() {
  const queryClient = useQueryClient();
  const {
    tracks,
    selectedTrackId,
    sortConfig,
    filter,
    setTracks,
    updateTrack,
    deleteTrack,
    selectTrack,
    setSortConfig,
    setFilter,
    clearFilter,
    clearStaleTracks,
    clearAllTracks,
  } = useTrackStore();

  // Periodically clean up stale tracks
  useEffect(() => {
    const interval = setInterval(() => {
      clearStaleTracks(STALE_TRACK_AGE_MS);
    }, STALE_CHECK_INTERVAL_MS);

    return () => clearInterval(interval);
  }, [clearStaleTracks]);

  // Fetch tracks from API
  const {
    data,
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: TRACKS_QUERY_KEY,
    queryFn: async () => {
      const response = await api.tracks.getAll();
      setTracks(response.data);
      return response.data;
    },
    refetchInterval: 10000, // Refetch every 10 seconds to sync with backend
    staleTime: 5000,
  });

  // Handle WebSocket track update
  const handleTrackUpdate = useCallback(
    (track: CorrelatedTrack) => {
      updateTrack(track);
      queryClient.setQueryData<CorrelatedTrack[]>(TRACKS_QUERY_KEY, (old) => {
        if (!old) return [track];
        const index = old.findIndex((t) => t.track_id === track.track_id);
        if (index >= 0) {
          const newTracks = [...old];
          newTracks[index] = track;
          return newTracks;
        }
        return [...old, track];
      });
    },
    [updateTrack, queryClient]
  );

  // Handle WebSocket track delete
  const handleTrackDelete = useCallback(
    (trackId: string) => {
      deleteTrack(trackId);
      queryClient.setQueryData<CorrelatedTrack[]>(TRACKS_QUERY_KEY, (old) => {
        if (!old) return [];
        return old.filter((t) => t.track_id !== trackId);
      });
    },
    [deleteTrack, queryClient]
  );

  // Get filtered and sorted tracks
  const getFilteredTracks = useCallback((): CorrelatedTrack[] => {
    let result = Array.from(tracks.values());

    // Apply filters (on the track data)
    if (filter.classification) {
      result = result.filter((item) => item.track.classification === filter.classification);
    }
    if (filter.threatLevel) {
      result = result.filter((item) => item.track.threat_level === filter.threatLevel);
    }
    if (filter.type) {
      result = result.filter((item) => item.track.type === filter.type);
    }
    if (filter.searchQuery) {
      const query = filter.searchQuery.toLowerCase();
      result = result.filter(
        (item) =>
          item.track.track_id.toLowerCase().includes(query) ||
          item.track.classification.toLowerCase().includes(query) ||
          item.track.type.toLowerCase().includes(query)
      );
    }

    // Apply sorting - handle 'insertedAt' specially since it's on TrackedItem, not track
    result.sort((a, b) => {
      let aValue: unknown;
      let bValue: unknown;

      if (sortConfig.key === 'insertedAt') {
        aValue = a.insertedAt;
        bValue = b.insertedAt;
      } else {
        aValue = getNestedValue(a.track, sortConfig.key);
        bValue = getNestedValue(b.track, sortConfig.key);
      }

      if (aValue === bValue) return 0;
      if (aValue === null || aValue === undefined) return 1;
      if (bValue === null || bValue === undefined) return -1;

      const comparison = aValue < bValue ? -1 : 1;
      return sortConfig.direction === 'asc' ? comparison : -comparison;
    });

    // Return just the track data
    return result.map((item) => item.track);
  }, [tracks, filter, sortConfig]);

  // Get selected track
  const getSelectedTrack = useCallback((): CorrelatedTrack | null => {
    if (!selectedTrackId) return null;
    return tracks.get(selectedTrackId)?.track || null;
  }, [tracks, selectedTrackId]);

  // Get track by ID
  const getTrackById = useCallback(
    (trackId: string): CorrelatedTrack | undefined => {
      return tracks.get(trackId)?.track;
    },
    [tracks]
  );

  // Toggle sort
  const toggleSort = useCallback(
    (key: string) => {
      if (sortConfig.key === key) {
        setSortConfig({
          key,
          direction: sortConfig.direction === 'asc' ? 'desc' : 'asc',
        });
      } else {
        setSortConfig({ key, direction: 'asc' });
      }
    },
    [sortConfig, setSortConfig]
  );

  return {
    // Data
    tracks: getFilteredTracks(),
    allTracks: Array.from(tracks.values()).map((item) => item.track),
    selectedTrack: getSelectedTrack(),
    selectedTrackId,
    trackCount: tracks.size,

    // State
    isLoading,
    error,
    sortConfig,
    filter,

    // Actions
    refetch,
    selectTrack,
    getTrackById,
    toggleSort,
    setFilter,
    clearFilter,
    clearAllTracks,

    // WebSocket handlers
    handleTrackUpdate,
    handleTrackDelete,
  };
}

// Helper to get nested object values by key path
function getNestedValue(obj: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce((current, key) => {
    return current && typeof current === 'object'
      ? (current as Record<string, unknown>)[key]
      : undefined;
  }, obj as unknown);
}

export default useTracks;
