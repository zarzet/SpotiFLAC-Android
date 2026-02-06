import 'dart:io';

import 'package:open_filex/open_filex.dart';
import 'package:spotiflac_android/services/platform_bridge.dart';
import 'package:spotiflac_android/utils/mime_utils.dart';

class FileAccessStat {
  final int? size;
  final DateTime? modified;

  const FileAccessStat({this.size, this.modified});
}

bool isContentUri(String? path) {
  return path != null && path.startsWith('content://');
}

Future<bool> fileExists(String? path) async {
  if (path == null || path.isEmpty) return false;
  if (isContentUri(path)) {
    return PlatformBridge.safExists(path);
  }
  return File(path).exists();
}

Future<void> deleteFile(String? path) async {
  if (path == null || path.isEmpty) return;
  if (isContentUri(path)) {
    await PlatformBridge.safDelete(path);
    return;
  }
  try {
    await File(path).delete();
  } catch (_) {}
}

Future<FileAccessStat?> fileStat(String? path) async {
  if (path == null || path.isEmpty) return null;
  if (isContentUri(path)) {
    final stat = await PlatformBridge.safStat(path);
    final exists = stat['exists'] as bool? ?? true;
    if (!exists) return null;
    return FileAccessStat(
      size: stat['size'] as int?,
      modified: stat['modified'] != null
          ? DateTime.fromMillisecondsSinceEpoch(stat['modified'] as int)
          : null,
    );
  }

  final stat = await FileStat.stat(path);
  if (stat.type == FileSystemEntityType.notFound) return null;
  return FileAccessStat(size: stat.size, modified: stat.modified);
}

Future<void> openFile(String path) async {
  if (isContentUri(path)) {
    await PlatformBridge.openContentUri(path, mimeType: '');
    return;
  }
  final mimeType = audioMimeTypeForPath(path);
  final result = await OpenFilex.open(path, type: mimeType);
  if (result.type != ResultType.done) {
    throw Exception(result.message);
  }
}
