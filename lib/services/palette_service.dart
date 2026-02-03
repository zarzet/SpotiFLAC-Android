import 'dart:io';
import 'package:flutter/material.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:palette_generator/palette_generator.dart';

/// Service for extracting dominant colors from images
/// Uses caching to avoid re-extraction and small image size for speed
class PaletteService {
  static final PaletteService instance = PaletteService._();
  PaletteService._();

  /// Cache for already computed colors
  final Map<String, Color> _colorCache = {};

  /// Extract dominant color from a network image URL
  /// Uses small image size and limited colors for speed
  Future<Color?> extractDominantColor(String? imageUrl) async {
    if (imageUrl == null || imageUrl.isEmpty) return null;
    if (!imageUrl.startsWith('http://') && !imageUrl.startsWith('https://')) {
      return null;
    }

    final cached = _colorCache[imageUrl];
    if (cached != null) {
      return cached;
    }

    try {
      final paletteGenerator = await PaletteGenerator.fromImageProvider(
        CachedNetworkImageProvider(imageUrl),
        size: const Size(64, 64),
        maximumColorCount: 8,
      );
      
      final color = paletteGenerator.dominantColor?.color ??
          paletteGenerator.vibrantColor?.color ??
          paletteGenerator.mutedColor?.color;
      
      if (color != null) {
        _colorCache[imageUrl] = color;
      }
      
      return color;
    } catch (e) {
      debugPrint('PaletteService error: $e');
      return null;
    }
  }

  /// Extract dominant color from a local file path
  Future<Color?> extractDominantColorFromFile(String? filePath) async {
    if (filePath == null || filePath.isEmpty) return null;

    final cached = _colorCache[filePath];
    if (cached != null) {
      return cached;
    }

    try {
      final file = File(filePath);
      if (!await file.exists()) return null;
      
      final paletteGenerator = await PaletteGenerator.fromImageProvider(
        FileImage(file),
        size: const Size(64, 64),
        maximumColorCount: 8,
      );
      
      final color = paletteGenerator.dominantColor?.color ??
          paletteGenerator.vibrantColor?.color ??
          paletteGenerator.mutedColor?.color;
      
      if (color != null) {
        _colorCache[filePath] = color;
      }
      
      return color;
    } catch (e) {
      debugPrint('PaletteService file error: $e');
      return null;
    }
  }

  /// Clear the color cache
  void clearCache() {
    _colorCache.clear();
  }

  /// Get cached color without computing
  Color? getCached(String? imageUrl) {
    if (imageUrl == null) return null;
    return _colorCache[imageUrl];
  }
}
