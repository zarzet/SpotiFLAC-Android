import 'dart:convert';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:spotiflac_android/models/track.dart';

const _collectionsStorageKey = 'library_collections_v1';

String trackCollectionKey(Track track) {
  final isrc = track.isrc?.trim();
  if (isrc != null && isrc.isNotEmpty) {
    return 'isrc:${isrc.toUpperCase()}';
  }
  final source = (track.source?.trim().isNotEmpty ?? false)
      ? track.source!.trim()
      : 'builtin';
  return '$source:${track.id}';
}

class CollectionTrackEntry {
  final String key;
  final Track track;
  final DateTime addedAt;

  const CollectionTrackEntry({
    required this.key,
    required this.track,
    required this.addedAt,
  });

  Map<String, dynamic> toJson() => {
    'key': key,
    'track': track.toJson(),
    'addedAt': addedAt.toIso8601String(),
  };

  factory CollectionTrackEntry.fromJson(Map<String, dynamic> json) {
    final addedAtRaw = json['addedAt'] as String?;
    return CollectionTrackEntry(
      key: json['key'] as String,
      track: Track.fromJson(Map<String, dynamic>.from(json['track'] as Map)),
      addedAt: DateTime.tryParse(addedAtRaw ?? '') ?? DateTime.now(),
    );
  }
}

class UserPlaylistCollection {
  final String id;
  final String name;
  final String? coverImagePath;
  final DateTime createdAt;
  final DateTime updatedAt;
  final List<CollectionTrackEntry> tracks;

  const UserPlaylistCollection({
    required this.id,
    required this.name,
    this.coverImagePath,
    required this.createdAt,
    required this.updatedAt,
    required this.tracks,
  });

  UserPlaylistCollection copyWith({
    String? id,
    String? name,
    String? Function()? coverImagePath,
    DateTime? createdAt,
    DateTime? updatedAt,
    List<CollectionTrackEntry>? tracks,
  }) {
    return UserPlaylistCollection(
      id: id ?? this.id,
      name: name ?? this.name,
      coverImagePath:
          coverImagePath != null ? coverImagePath() : this.coverImagePath,
      createdAt: createdAt ?? this.createdAt,
      updatedAt: updatedAt ?? this.updatedAt,
      tracks: tracks ?? this.tracks,
    );
  }

  bool containsTrack(Track track) {
    final key = trackCollectionKey(track);
    return tracks.any((entry) => entry.key == key);
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    if (coverImagePath != null) 'coverImagePath': coverImagePath,
    'createdAt': createdAt.toIso8601String(),
    'updatedAt': updatedAt.toIso8601String(),
    'tracks': tracks.map((e) => e.toJson()).toList(),
  };

  factory UserPlaylistCollection.fromJson(Map<String, dynamic> json) {
    final createdAtRaw = json['createdAt'] as String?;
    final updatedAtRaw = json['updatedAt'] as String?;
    final createdAt = DateTime.tryParse(createdAtRaw ?? '') ?? DateTime.now();
    final updatedAt = DateTime.tryParse(updatedAtRaw ?? '') ?? createdAt;
    final tracksRaw = (json['tracks'] as List?) ?? const [];
    return UserPlaylistCollection(
      id: json['id'] as String,
      name: json['name'] as String? ?? '',
      coverImagePath: json['coverImagePath'] as String?,
      createdAt: createdAt,
      updatedAt: updatedAt,
      tracks: tracksRaw
          .whereType<Map>()
          .map(
            (e) => CollectionTrackEntry.fromJson(Map<String, dynamic>.from(e)),
          )
          .toList(growable: false),
    );
  }
}

class LibraryCollectionsState {
  final List<CollectionTrackEntry> wishlist;
  final List<CollectionTrackEntry> loved;
  final List<UserPlaylistCollection> playlists;
  final bool isLoaded;

  const LibraryCollectionsState({
    this.wishlist = const [],
    this.loved = const [],
    this.playlists = const [],
    this.isLoaded = false,
  });

  int get wishlistCount => wishlist.length;
  int get lovedCount => loved.length;
  int get playlistCount => playlists.length;

  bool isInWishlist(Track track) {
    final key = trackCollectionKey(track);
    return wishlist.any((entry) => entry.key == key);
  }

  bool isLoved(Track track) {
    final key = trackCollectionKey(track);
    return loved.any((entry) => entry.key == key);
  }

  UserPlaylistCollection? playlistById(String playlistId) {
    for (final playlist in playlists) {
      if (playlist.id == playlistId) return playlist;
    }
    return null;
  }

  LibraryCollectionsState copyWith({
    List<CollectionTrackEntry>? wishlist,
    List<CollectionTrackEntry>? loved,
    List<UserPlaylistCollection>? playlists,
    bool? isLoaded,
  }) {
    return LibraryCollectionsState(
      wishlist: wishlist ?? this.wishlist,
      loved: loved ?? this.loved,
      playlists: playlists ?? this.playlists,
      isLoaded: isLoaded ?? this.isLoaded,
    );
  }

  Map<String, dynamic> toJson() => {
    'wishlist': wishlist.map((e) => e.toJson()).toList(),
    'loved': loved.map((e) => e.toJson()).toList(),
    'playlists': playlists.map((e) => e.toJson()).toList(),
  };

  factory LibraryCollectionsState.fromJson(Map<String, dynamic> json) {
    final wishlistRaw = (json['wishlist'] as List?) ?? const [];
    final lovedRaw = (json['loved'] as List?) ?? const [];
    final playlistsRaw = (json['playlists'] as List?) ?? const [];

    return LibraryCollectionsState(
      wishlist: wishlistRaw
          .whereType<Map>()
          .map(
            (e) => CollectionTrackEntry.fromJson(Map<String, dynamic>.from(e)),
          )
          .toList(growable: false),
      loved: lovedRaw
          .whereType<Map>()
          .map(
            (e) => CollectionTrackEntry.fromJson(Map<String, dynamic>.from(e)),
          )
          .toList(growable: false),
      playlists: playlistsRaw
          .whereType<Map>()
          .map(
            (e) =>
                UserPlaylistCollection.fromJson(Map<String, dynamic>.from(e)),
          )
          .toList(growable: false),
      isLoaded: true,
    );
  }
}

class LibraryCollectionsNotifier extends Notifier<LibraryCollectionsState> {
  final Future<SharedPreferences> _prefs = SharedPreferences.getInstance();
  Future<void>? _loadFuture;

  @override
  LibraryCollectionsState build() {
    _loadFuture = _load();
    return const LibraryCollectionsState();
  }

  Future<void> _load() async {
    final prefs = await _prefs;
    final raw = prefs.getString(_collectionsStorageKey);

    if (raw == null || raw.isEmpty) {
      state = state.copyWith(isLoaded: true);
      return;
    }

    try {
      final parsed = jsonDecode(raw);
      if (parsed is Map<String, dynamic>) {
        state = LibraryCollectionsState.fromJson(parsed);
      } else {
        state = state.copyWith(isLoaded: true);
      }
    } catch (_) {
      state = state.copyWith(isLoaded: true);
    }
  }

  Future<void> _save() async {
    final prefs = await _prefs;
    await prefs.setString(_collectionsStorageKey, jsonEncode(state.toJson()));
  }

  Future<void> _ensureLoaded() async {
    if (state.isLoaded) return;
    await (_loadFuture ?? _load());
  }

  Future<bool> toggleWishlist(Track track) async {
    await _ensureLoaded();
    final key = trackCollectionKey(track);
    final index = state.wishlist.indexWhere((entry) => entry.key == key);

    if (index >= 0) {
      final updated = [...state.wishlist]..removeAt(index);
      state = state.copyWith(wishlist: updated);
      await _save();
      return false;
    }

    final entry = CollectionTrackEntry(
      key: key,
      track: track,
      addedAt: DateTime.now(),
    );
    final updated = [entry, ...state.wishlist];
    state = state.copyWith(wishlist: updated);
    await _save();
    return true;
  }

  Future<bool> toggleLoved(Track track) async {
    await _ensureLoaded();
    final key = trackCollectionKey(track);
    final index = state.loved.indexWhere((entry) => entry.key == key);

    if (index >= 0) {
      final updated = [...state.loved]..removeAt(index);
      state = state.copyWith(loved: updated);
      await _save();
      return false;
    }

    final entry = CollectionTrackEntry(
      key: key,
      track: track,
      addedAt: DateTime.now(),
    );
    final updated = [entry, ...state.loved];
    state = state.copyWith(loved: updated);
    await _save();
    return true;
  }

  Future<void> removeFromWishlist(String trackKey) async {
    await _ensureLoaded();
    final updated = state.wishlist
        .where((entry) => entry.key != trackKey)
        .toList(growable: false);
    if (updated.length == state.wishlist.length) return;
    state = state.copyWith(wishlist: updated);
    await _save();
  }

  Future<void> removeFromLoved(String trackKey) async {
    await _ensureLoaded();
    final updated = state.loved
        .where((entry) => entry.key != trackKey)
        .toList(growable: false);
    if (updated.length == state.loved.length) return;
    state = state.copyWith(loved: updated);
    await _save();
  }

  Future<String> createPlaylist(String name) async {
    await _ensureLoaded();
    final now = DateTime.now();
    final id = 'pl_${now.microsecondsSinceEpoch}';
    final trimmedName = name.trim();

    final playlist = UserPlaylistCollection(
      id: id,
      name: trimmedName,
      createdAt: now,
      updatedAt: now,
      tracks: const [],
    );

    state = state.copyWith(playlists: [playlist, ...state.playlists]);
    await _save();
    return id;
  }

  Future<void> renamePlaylist(String playlistId, String newName) async {
    await _ensureLoaded();
    final trimmed = newName.trim();
    if (trimmed.isEmpty) return;

    final now = DateTime.now();
    final updated = state.playlists
        .map((playlist) {
          if (playlist.id != playlistId) return playlist;
          return playlist.copyWith(name: trimmed, updatedAt: now);
        })
        .toList(growable: false);

    state = state.copyWith(playlists: updated);
    await _save();
  }

  Future<void> deletePlaylist(String playlistId) async {
    await _ensureLoaded();
    final updated = state.playlists
        .where((playlist) => playlist.id != playlistId)
        .toList(growable: false);
    if (updated.length == state.playlists.length) return;
    state = state.copyWith(playlists: updated);
    await _save();
  }

  Future<bool> addTrackToPlaylist(String playlistId, Track track) async {
    await _ensureLoaded();
    final key = trackCollectionKey(track);
    final now = DateTime.now();
    var changed = false;

    final updated = state.playlists
        .map((playlist) {
          if (playlist.id != playlistId) return playlist;
          final alreadyInPlaylist = playlist.tracks.any(
            (entry) => entry.key == key,
          );
          if (alreadyInPlaylist) return playlist;
          changed = true;
          final entry = CollectionTrackEntry(
            key: key,
            track: track,
            addedAt: now,
          );
          return playlist.copyWith(
            tracks: [entry, ...playlist.tracks],
            updatedAt: now,
          );
        })
        .toList(growable: false);

    if (!changed) return false;

    state = state.copyWith(playlists: updated);
    await _save();
    return true;
  }

  Future<void> removeTrackFromPlaylist(
    String playlistId,
    String trackKey,
  ) async {
    await _ensureLoaded();
    final now = DateTime.now();
    var changed = false;

    final updated = state.playlists
        .map((playlist) {
          if (playlist.id != playlistId) return playlist;
          final nextTracks = playlist.tracks
              .where((entry) => entry.key != trackKey)
              .toList(growable: false);
          if (nextTracks.length == playlist.tracks.length) return playlist;
          changed = true;
          return playlist.copyWith(tracks: nextTracks, updatedAt: now);
        })
        .toList(growable: false);

    if (!changed) return;

    state = state.copyWith(playlists: updated);
    await _save();
  }

  /// Returns the directory for storing playlist cover images, creating it
  /// if necessary.
  Future<Directory> _playlistCoversDir() async {
    final appDir = await getApplicationSupportDirectory();
    final dir = Directory(p.join(appDir.path, 'playlist_covers'));
    if (!await dir.exists()) {
      await dir.create(recursive: true);
    }
    return dir;
  }

  /// Sets a custom cover image for a playlist by copying the source file
  /// into the app's persistent storage.
  Future<void> setPlaylistCover(
    String playlistId,
    String sourceFilePath,
  ) async {
    await _ensureLoaded();
    final coversDir = await _playlistCoversDir();
    final ext = p.extension(sourceFilePath).toLowerCase();
    final destPath = p.join(coversDir.path, '$playlistId$ext');

    // Copy image to persistent location
    await File(sourceFilePath).copy(destPath);

    final now = DateTime.now();
    final updated = state.playlists
        .map((playlist) {
          if (playlist.id != playlistId) return playlist;
          return playlist.copyWith(
            coverImagePath: () => destPath,
            updatedAt: now,
          );
        })
        .toList(growable: false);

    state = state.copyWith(playlists: updated);
    await _save();
  }

  /// Removes the custom cover image for a playlist (falls back to first
  /// track's cover).
  Future<void> removePlaylistCover(String playlistId) async {
    await _ensureLoaded();
    final playlist = state.playlistById(playlistId);
    if (playlist == null) return;

    // Delete the file if it exists
    final path = playlist.coverImagePath;
    if (path != null) {
      final file = File(path);
      if (await file.exists()) {
        await file.delete();
      }
    }

    final now = DateTime.now();
    final updated = state.playlists
        .map((pl) {
          if (pl.id != playlistId) return pl;
          return pl.copyWith(coverImagePath: () => null, updatedAt: now);
        })
        .toList(growable: false);

    state = state.copyWith(playlists: updated);
    await _save();
  }
}

final libraryCollectionsProvider =
    NotifierProvider<LibraryCollectionsNotifier, LibraryCollectionsState>(
      LibraryCollectionsNotifier.new,
    );
