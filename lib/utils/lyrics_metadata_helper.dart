import 'dart:io';

import 'package:spotiflac_android/services/platform_bridge.dart';
import 'package:spotiflac_android/utils/file_access.dart';

bool hasEmbeddedLyricsMetadata(Map<String, String> metadata) {
  final lyrics = (metadata['LYRICS'] ?? '').trim();
  if (lyrics.isNotEmpty) return true;

  final unsyncedLyrics = (metadata['UNSYNCEDLYRICS'] ?? '').trim();
  if (unsyncedLyrics.isNotEmpty) return true;

  return false;
}

String _sidecarLrcPath(String path) {
  final slash = path.lastIndexOf(Platform.pathSeparator);
  final dot = path.lastIndexOf('.');
  if (dot > slash) {
    return '${path.substring(0, dot)}.lrc';
  }
  return '$path.lrc';
}

Future<void> ensureLyricsMetadataForConversion({
  required Map<String, String> metadata,
  required String sourcePath,
  required bool shouldEmbedLyrics,
  required String trackName,
  required String artistName,
  String spotifyId = '',
  int durationMs = 0,
}) async {
  if (!shouldEmbedLyrics || hasEmbeddedLyricsMetadata(metadata)) {
    return;
  }

  String? lyrics;

  // Prefer sidecar .lrc when available to avoid network calls.
  if (!isContentUri(sourcePath)) {
    try {
      final lrcPath = _sidecarLrcPath(sourcePath);
      final lrcFile = File(lrcPath);
      if (await lrcFile.exists()) {
        final content = (await lrcFile.readAsString()).trim();
        if (content.isNotEmpty) {
          lyrics = content;
        }
      }
    } catch (_) {}
  }

  if (lyrics == null || lyrics.isEmpty) {
    try {
      final fetched = await PlatformBridge.getLyricsLRC(
        spotifyId,
        trackName,
        artistName,
        durationMs: durationMs,
      );
      final normalized = fetched.trim();
      if (normalized.isNotEmpty &&
          normalized.toLowerCase() != '[instrumental:true]') {
        lyrics = normalized;
      }
    } catch (_) {}
  }

  if (lyrics == null || lyrics.isEmpty) {
    return;
  }

  metadata['LYRICS'] = lyrics;
  metadata['UNSYNCEDLYRICS'] = lyrics;
}
