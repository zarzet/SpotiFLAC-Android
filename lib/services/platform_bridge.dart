import 'dart:convert';
import 'package:flutter/services.dart';
import 'package:spotiflac_android/utils/logger.dart';

final _log = AppLogger('PlatformBridge');

/// Bridge to communicate with Go backend via platform channels
class PlatformBridge {
  static const _channel = MethodChannel('com.zarz.spotiflac/backend');

  /// Parse and validate Spotify URL
  static Future<Map<String, dynamic>> parseSpotifyUrl(String url) async {
    _log.d('parseSpotifyUrl: $url');
    final result = await _channel.invokeMethod('parseSpotifyUrl', {'url': url});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Get Spotify metadata from URL
  static Future<Map<String, dynamic>> getSpotifyMetadata(String url) async {
    _log.d('getSpotifyMetadata: $url');
    final result = await _channel.invokeMethod('getSpotifyMetadata', {'url': url});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Search Spotify
  static Future<Map<String, dynamic>> searchSpotify(String query, {int limit = 10}) async {
    _log.d('searchSpotify: "$query" (limit: $limit)');
    final result = await _channel.invokeMethod('searchSpotify', {
      'query': query,
      'limit': limit,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Search Spotify for tracks and artists
  static Future<Map<String, dynamic>> searchSpotifyAll(String query, {int trackLimit = 15, int artistLimit = 3}) async {
    _log.d('searchSpotifyAll: "$query"');
    final result = await _channel.invokeMethod('searchSpotifyAll', {
      'query': query,
      'track_limit': trackLimit,
      'artist_limit': artistLimit,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Check track availability on streaming services
  static Future<Map<String, dynamic>> checkAvailability(String spotifyId, String isrc) async {
    _log.d('checkAvailability: $spotifyId (ISRC: $isrc)');
    final result = await _channel.invokeMethod('checkAvailability', {
      'spotify_id': spotifyId,
      'isrc': isrc,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Download a track from specific service
  static Future<Map<String, dynamic>> downloadTrack({
    required String isrc,
    required String service,
    required String spotifyId,
    required String trackName,
    required String artistName,
    required String albumName,
    String? albumArtist,
    String? coverUrl,
    required String outputDir,
    required String filenameFormat,
    String quality = 'LOSSLESS',
    bool embedLyrics = true,
    bool embedMaxQualityCover = true,
    int trackNumber = 1,
    int discNumber = 1,
    int totalTracks = 1,
    String? releaseDate,
    String? itemId,
    int durationMs = 0,
  }) async {
    _log.i('downloadTrack: "$trackName" by $artistName via $service');
    final request = jsonEncode({
      'isrc': isrc,
      'service': service,
      'spotify_id': spotifyId,
      'track_name': trackName,
      'artist_name': artistName,
      'album_name': albumName,
      'album_artist': albumArtist ?? artistName,
      'cover_url': coverUrl,
      'output_dir': outputDir,
      'filename_format': filenameFormat,
      'quality': quality,
      'embed_lyrics': embedLyrics,
      'embed_max_quality_cover': embedMaxQualityCover,
      'track_number': trackNumber,
      'disc_number': discNumber,
      'total_tracks': totalTracks,
      'release_date': releaseDate ?? '',
      'item_id': itemId ?? '',
      'duration_ms': durationMs,
    });
    
    final result = await _channel.invokeMethod('downloadTrack', request);
    final response = jsonDecode(result as String) as Map<String, dynamic>;
    if (response['success'] == true) {
      _log.i('Download success: ${response['file_path']}');
    } else {
      _log.w('Download failed: ${response['error']}');
    }
    return response;
  }

  /// Download with automatic fallback to other services
  static Future<Map<String, dynamic>> downloadWithFallback({
    required String isrc,
    required String spotifyId,
    required String trackName,
    required String artistName,
    required String albumName,
    String? albumArtist,
    String? coverUrl,
    required String outputDir,
    required String filenameFormat,
    String quality = 'LOSSLESS',
    bool embedLyrics = true,
    bool embedMaxQualityCover = true,
    int trackNumber = 1,
    int discNumber = 1,
    int totalTracks = 1,
    String? releaseDate,
    String preferredService = 'tidal',
    String? itemId,
    int durationMs = 0,
  }) async {
    _log.i('downloadWithFallback: "$trackName" by $artistName (preferred: $preferredService)');
    final request = jsonEncode({
      'isrc': isrc,
      'service': preferredService,
      'spotify_id': spotifyId,
      'track_name': trackName,
      'artist_name': artistName,
      'album_name': albumName,
      'album_artist': albumArtist ?? artistName,
      'cover_url': coverUrl,
      'output_dir': outputDir,
      'filename_format': filenameFormat,
      'quality': quality,
      'embed_lyrics': embedLyrics,
      'embed_max_quality_cover': embedMaxQualityCover,
      'track_number': trackNumber,
      'disc_number': discNumber,
      'total_tracks': totalTracks,
      'release_date': releaseDate ?? '',
      'item_id': itemId ?? '',
      'duration_ms': durationMs,
    });
    
    final result = await _channel.invokeMethod('downloadWithFallback', request);
    final response = jsonDecode(result as String) as Map<String, dynamic>;
    if (response['success'] == true) {
      final service = response['service'] ?? 'unknown';
      final filePath = response['file_path'] ?? '';
      final bitDepth = response['actual_bit_depth'];
      final sampleRate = response['actual_sample_rate'];
      final qualityStr = bitDepth != null && sampleRate != null 
          ? ' ($bitDepth-bit/${(sampleRate / 1000).toStringAsFixed(1)}kHz)'
          : '';
      _log.i('Download success via $service$qualityStr: $filePath');
    } else {
      final error = response['error'] ?? 'Unknown error';
      final errorType = response['error_type'] ?? '';
      _log.e('Download failed: $error (type: $errorType)');
    }
    return response;
  }

  /// Get download progress (legacy single download)
  static Future<Map<String, dynamic>> getDownloadProgress() async {
    final result = await _channel.invokeMethod('getDownloadProgress');
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Get progress for all active downloads (concurrent mode)
  static Future<Map<String, dynamic>> getAllDownloadProgress() async {
    final result = await _channel.invokeMethod('getAllDownloadProgress');
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Initialize progress tracking for a download item
  static Future<void> initItemProgress(String itemId) async {
    await _channel.invokeMethod('initItemProgress', {'item_id': itemId});
  }

  /// Finish progress tracking for a download item
  static Future<void> finishItemProgress(String itemId) async {
    await _channel.invokeMethod('finishItemProgress', {'item_id': itemId});
  }

  /// Clear progress tracking for a download item
  static Future<void> clearItemProgress(String itemId) async {
    await _channel.invokeMethod('clearItemProgress', {'item_id': itemId});
  }

  /// Set download directory
  static Future<void> setDownloadDirectory(String path) async {
    await _channel.invokeMethod('setDownloadDirectory', {'path': path});
  }

  /// Check if file with ISRC already exists
  static Future<Map<String, dynamic>> checkDuplicate(String outputDir, String isrc) async {
    final result = await _channel.invokeMethod('checkDuplicate', {
      'output_dir': outputDir,
      'isrc': isrc,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Build filename from template
  static Future<String> buildFilename(String template, Map<String, dynamic> metadata) async {
    final result = await _channel.invokeMethod('buildFilename', {
      'template': template,
      'metadata': jsonEncode(metadata),
    });
    return result as String;
  }

  /// Sanitize filename
  static Future<String> sanitizeFilename(String filename) async {
    final result = await _channel.invokeMethod('sanitizeFilename', {
      'filename': filename,
    });
    return result as String;
  }

  /// Fetch lyrics for a track
  static Future<Map<String, dynamic>> fetchLyrics(
    String spotifyId,
    String trackName,
    String artistName,
  ) async {
    final result = await _channel.invokeMethod('fetchLyrics', {
      'spotify_id': spotifyId,
      'track_name': trackName,
      'artist_name': artistName,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Get lyrics in LRC format
  /// First tries to extract from embedded file, then falls back to internet
  static Future<String> getLyricsLRC(
    String spotifyId,
    String trackName,
    String artistName, {
    String? filePath,
  }) async {
    final result = await _channel.invokeMethod('getLyricsLRC', {
      'spotify_id': spotifyId,
      'track_name': trackName,
      'artist_name': artistName,
      'file_path': filePath ?? '',
    });
    return result as String;
  }

  /// Embed lyrics into an existing FLAC file
  static Future<Map<String, dynamic>> embedLyricsToFile(
    String filePath,
    String lyrics,
  ) async {
    final result = await _channel.invokeMethod('embedLyricsToFile', {
      'file_path': filePath,
      'lyrics': lyrics,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Cleanup idle HTTP connections to prevent TCP exhaustion
  /// Call this periodically during large batch downloads
  static Future<void> cleanupConnections() async {
    await _channel.invokeMethod('cleanupConnections');
  }

  /// Read metadata directly from a FLAC file
  /// Returns all embedded metadata (title, artist, album, track number, etc.)
  /// This reads from the actual file, not from cached/database data
  static Future<Map<String, dynamic>> readFileMetadata(String filePath) async {
    final result = await _channel.invokeMethod('readFileMetadata', {
      'file_path': filePath,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Start foreground download service to keep downloads running in background
  static Future<void> startDownloadService({
    String trackName = '',
    String artistName = '',
    int queueCount = 0,
  }) async {
    await _channel.invokeMethod('startDownloadService', {
      'track_name': trackName,
      'artist_name': artistName,
      'queue_count': queueCount,
    });
  }

  /// Stop foreground download service
  static Future<void> stopDownloadService() async {
    await _channel.invokeMethod('stopDownloadService');
  }

  /// Update download service notification progress
  static Future<void> updateDownloadServiceProgress({
    required String trackName,
    required String artistName,
    required int progress,
    required int total,
    required int queueCount,
  }) async {
    await _channel.invokeMethod('updateDownloadServiceProgress', {
      'track_name': trackName,
      'artist_name': artistName,
      'progress': progress,
      'total': total,
      'queue_count': queueCount,
    });
  }

  /// Check if download service is running
  static Future<bool> isDownloadServiceRunning() async {
    final result = await _channel.invokeMethod('isDownloadServiceRunning');
    return result as bool;
  }

  /// Set custom Spotify API credentials
  /// Pass empty strings to use default credentials
  static Future<void> setSpotifyCredentials(String clientId, String clientSecret) async {
    await _channel.invokeMethod('setSpotifyCredentials', {
      'client_id': clientId,
      'client_secret': clientSecret,
    });
  }

  /// Pre-warm track ID cache for album/playlist tracks
  /// This runs in background and returns immediately
  /// Speeds up subsequent downloads by caching ISRC → Track ID mappings
  static Future<void> preWarmTrackCache(List<Map<String, String>> tracks) async {
    final tracksJson = jsonEncode(tracks);
    await _channel.invokeMethod('preWarmTrackCache', {'tracks': tracksJson});
  }

  /// Get current track cache size
  static Future<int> getTrackCacheSize() async {
    final result = await _channel.invokeMethod('getTrackCacheSize');
    return result as int;
  }

  /// Clear track ID cache
  static Future<void> clearTrackCache() async {
    await _channel.invokeMethod('clearTrackCache');
  }

  // ==================== DEEZER API ====================

  /// Search Deezer for tracks and artists (no API key required)
  static Future<Map<String, dynamic>> searchDeezerAll(String query, {int trackLimit = 15, int artistLimit = 3}) async {
    final result = await _channel.invokeMethod('searchDeezerAll', {
      'query': query,
      'track_limit': trackLimit,
      'artist_limit': artistLimit,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Get Deezer metadata by type and ID
  static Future<Map<String, dynamic>> getDeezerMetadata(String resourceType, String resourceId) async {
    final result = await _channel.invokeMethod('getDeezerMetadata', {
      'resource_type': resourceType,
      'resource_id': resourceId,
    });
    if (result == null) {
      throw Exception('getDeezerMetadata returned null for $resourceType:$resourceId');
    }
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Parse Deezer URL and return type and ID
  static Future<Map<String, dynamic>> parseDeezerUrl(String url) async {
    final result = await _channel.invokeMethod('parseDeezerUrl', {'url': url});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Search Deezer by ISRC
  static Future<Map<String, dynamic>> searchDeezerByISRC(String isrc) async {
    final result = await _channel.invokeMethod('searchDeezerByISRC', {'isrc': isrc});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Convert Spotify track to Deezer and get metadata (for rate limit fallback)
  static Future<Map<String, dynamic>> convertSpotifyToDeezer(String resourceType, String spotifyId) async {
    final result = await _channel.invokeMethod('convertSpotifyToDeezer', {
      'resource_type': resourceType,
      'spotify_id': spotifyId,
    });
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Get Spotify metadata with automatic Deezer fallback on rate limit
  static Future<Map<String, dynamic>> getSpotifyMetadataWithFallback(String url) async {
    final result = await _channel.invokeMethod('getSpotifyMetadataWithFallback', {'url': url});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  // ==================== GO BACKEND LOGS ====================

  /// Get all logs from Go backend
  static Future<List<Map<String, dynamic>>> getGoLogs() async {
    final result = await _channel.invokeMethod('getLogs');
    final logs = jsonDecode(result as String) as List<dynamic>;
    return logs.map((e) => e as Map<String, dynamic>).toList();
  }

  /// Get logs since a specific index (for incremental updates)
  static Future<Map<String, dynamic>> getGoLogsSince(int index) async {
    final result = await _channel.invokeMethod('getLogsSince', {'index': index});
    return jsonDecode(result as String) as Map<String, dynamic>;
  }

  /// Clear Go backend logs
  static Future<void> clearGoLogs() async {
    await _channel.invokeMethod('clearLogs');
  }

  /// Get Go backend log count
  static Future<int> getGoLogCount() async {
    final result = await _channel.invokeMethod('getLogCount');
    return result as int;
  }

  /// Enable or disable Go backend logging
  static Future<void> setGoLoggingEnabled(bool enabled) async {
    await _channel.invokeMethod('setLoggingEnabled', {'enabled': enabled});
  }
}
