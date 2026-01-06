import 'package:flutter/material.dart';

/// Storage keys for theme settings persistence
const String kThemeModeKey = 'theme_mode';
const String kUseDynamicColorKey = 'use_dynamic_color';
const String kSeedColorKey = 'seed_color';
const String kUseAmoledKey = 'use_amoled';

/// Default Spotify green color for fallback
const int kDefaultSeedColor = 0xFF1DB954;

/// Theme settings model for Material Expressive 3
class ThemeSettings {
  final ThemeMode themeMode;
  final bool useDynamicColor;
  final int seedColorValue;
  final bool useAmoled; // Pure black background for OLED screens

  const ThemeSettings({
    this.themeMode = ThemeMode.system,
    this.useDynamicColor = true,
    this.seedColorValue = kDefaultSeedColor,
    this.useAmoled = false,
  });

  /// Get seed color as Color object
  Color get seedColor => Color(seedColorValue);

  /// Create a copy with updated values
  ThemeSettings copyWith({
    ThemeMode? themeMode,
    bool? useDynamicColor,
    int? seedColorValue,
    bool? useAmoled,
  }) {
    return ThemeSettings(
      themeMode: themeMode ?? this.themeMode,
      useDynamicColor: useDynamicColor ?? this.useDynamicColor,
      seedColorValue: seedColorValue ?? this.seedColorValue,
      useAmoled: useAmoled ?? this.useAmoled,
    );
  }

  /// Convert to JSON map for persistence
  Map<String, dynamic> toJson() => {
        kThemeModeKey: themeMode.name,
        kUseDynamicColorKey: useDynamicColor,
        kSeedColorKey: seedColorValue,
        kUseAmoledKey: useAmoled,
      };

  /// Create from JSON map
  factory ThemeSettings.fromJson(Map<String, dynamic> json) {
    return ThemeSettings(
      themeMode: _themeModeFromString(json[kThemeModeKey] as String?),
      useDynamicColor: json[kUseDynamicColorKey] as bool? ?? true,
      seedColorValue: json[kSeedColorKey] as int? ?? kDefaultSeedColor,
      useAmoled: json[kUseAmoledKey] as bool? ?? false,
    );
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is ThemeSettings &&
        other.themeMode == themeMode &&
        other.useDynamicColor == useDynamicColor &&
        other.seedColorValue == seedColorValue &&
        other.useAmoled == useAmoled;
  }

  @override
  int get hashCode =>
      themeMode.hashCode ^ useDynamicColor.hashCode ^ seedColorValue.hashCode ^ useAmoled.hashCode;
}

/// Helper to convert string to ThemeMode
ThemeMode _themeModeFromString(String? value) {
  if (value == null) return ThemeMode.system;
  return ThemeMode.values.firstWhere(
    (e) => e.name == value,
    orElse: () => ThemeMode.system,
  );
}
