import 'package:flutter/material.dart';
import 'package:spotiflac_android/models/theme_settings.dart';

/// App theme configuration for Material Expressive 3
class AppTheme {
  /// Default seed color (Spotify green)
  static const Color defaultSeedColor = Color(kDefaultSeedColor);

  /// Create light theme
  static ThemeData light({
    ColorScheme? dynamicScheme,
    Color? seedColor,
  }) {
    final scheme = dynamicScheme ??
        ColorScheme.fromSeed(
          seedColor: seedColor ?? defaultSeedColor,
          brightness: Brightness.light,
        );

    return ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      appBarTheme: _appBarTheme(scheme),
      cardTheme: _cardTheme(scheme),
      elevatedButtonTheme: _elevatedButtonTheme(scheme),
      filledButtonTheme: _filledButtonTheme(scheme),
      outlinedButtonTheme: _outlinedButtonTheme(scheme),
      textButtonTheme: _textButtonTheme(scheme),
      floatingActionButtonTheme: _fabTheme(scheme),
      inputDecorationTheme: _inputDecorationTheme(scheme),
      listTileTheme: _listTileTheme(scheme),
      dialogTheme: _dialogTheme(scheme),
      navigationBarTheme: _navigationBarTheme(scheme),
      snackBarTheme: _snackBarTheme(scheme),
      progressIndicatorTheme: _progressIndicatorTheme(scheme),
      switchTheme: _switchTheme(scheme),
      chipTheme: _chipTheme(scheme),
      dividerTheme: _dividerTheme(scheme),
    );
  }

  /// Create dark theme
  static ThemeData dark({
    ColorScheme? dynamicScheme,
    Color? seedColor,
    bool isAmoled = false,
  }) {
    final scheme = dynamicScheme ??
        ColorScheme.fromSeed(
          seedColor: seedColor ?? defaultSeedColor,
          brightness: Brightness.dark,
        );

    return ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      scaffoldBackgroundColor: isAmoled ? Colors.black : null,
      appBarTheme: _appBarTheme(scheme, isAmoled: isAmoled),
      cardTheme: _cardTheme(scheme),
      elevatedButtonTheme: _elevatedButtonTheme(scheme),
      filledButtonTheme: _filledButtonTheme(scheme),
      outlinedButtonTheme: _outlinedButtonTheme(scheme),
      textButtonTheme: _textButtonTheme(scheme),
      floatingActionButtonTheme: _fabTheme(scheme),
      inputDecorationTheme: _inputDecorationTheme(scheme),
      listTileTheme: _listTileTheme(scheme),
      dialogTheme: _dialogTheme(scheme),
      navigationBarTheme: _navigationBarTheme(scheme, isAmoled: isAmoled),
      snackBarTheme: _snackBarTheme(scheme),
      progressIndicatorTheme: _progressIndicatorTheme(scheme),
      switchTheme: _switchTheme(scheme),
      chipTheme: _chipTheme(scheme),
      dividerTheme: _dividerTheme(scheme),
    );
  }

  /// AppBar theme
  static AppBarTheme _appBarTheme(ColorScheme scheme, {bool isAmoled = false}) => AppBarTheme(
        elevation: 0,
        scrolledUnderElevation: isAmoled ? 0 : 3,
        backgroundColor: isAmoled ? Colors.black : scheme.surface,
        foregroundColor: scheme.onSurface,
        surfaceTintColor: isAmoled ? Colors.transparent : scheme.surfaceTint,
        centerTitle: true,
        titleTextStyle: TextStyle(
          color: scheme.onSurface,
          fontSize: 22,
          fontWeight: FontWeight.w500,
        ),
      );

  /// Card theme
  static CardThemeData _cardTheme(ColorScheme scheme) => CardThemeData(
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        color: scheme.surfaceContainerLow,
        surfaceTintColor: scheme.surfaceTint,
      );

  /// Elevated button theme
  static ElevatedButtonThemeData _elevatedButtonTheme(ColorScheme scheme) =>
      ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          elevation: 1,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
        ),
      );

  /// Filled button theme
  static FilledButtonThemeData _filledButtonTheme(ColorScheme scheme) =>
      FilledButtonThemeData(
        style: FilledButton.styleFrom(
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
        ),
      );

  /// Outlined button theme
  static OutlinedButtonThemeData _outlinedButtonTheme(ColorScheme scheme) =>
      OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
        ),
      );

  /// Text button theme
  static TextButtonThemeData _textButtonTheme(ColorScheme scheme) =>
      TextButtonThemeData(
        style: TextButton.styleFrom(
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        ),
      );

  /// FAB theme
  static FloatingActionButtonThemeData _fabTheme(ColorScheme scheme) =>
      FloatingActionButtonThemeData(
        elevation: 3,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        backgroundColor: scheme.primaryContainer,
        foregroundColor: scheme.onPrimaryContainer,
      );

  /// Input decoration theme
  static InputDecorationTheme _inputDecorationTheme(ColorScheme scheme) =>
      InputDecorationTheme(
        filled: true,
        fillColor: scheme.surfaceContainerHighest,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: scheme.primary, width: 2),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: scheme.error, width: 1),
        ),
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      );

  /// List tile theme
  static ListTileThemeData _listTileTheme(ColorScheme scheme) => ListTileThemeData(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      );

  /// Dialog theme
  static DialogThemeData _dialogTheme(ColorScheme scheme) => DialogThemeData(
        elevation: 6,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(28)),
        backgroundColor: scheme.surfaceContainerHigh,
        surfaceTintColor: scheme.surfaceTint,
      );

  /// Navigation bar theme
  static NavigationBarThemeData _navigationBarTheme(ColorScheme scheme, {bool isAmoled = false}) =>
      NavigationBarThemeData(
        elevation: 0,
        backgroundColor: isAmoled ? Colors.black : scheme.surfaceContainer,
        indicatorColor: scheme.secondaryContainer,
        surfaceTintColor: isAmoled ? Colors.transparent : scheme.surfaceTint,
        labelBehavior: NavigationDestinationLabelBehavior.alwaysShow,
      );

  /// SnackBar theme
  static SnackBarThemeData _snackBarTheme(ColorScheme scheme) => SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        backgroundColor: scheme.inverseSurface,
        contentTextStyle: TextStyle(color: scheme.onInverseSurface),
      );

  /// Progress indicator theme
  static ProgressIndicatorThemeData _progressIndicatorTheme(ColorScheme scheme) =>
      ProgressIndicatorThemeData(
        color: scheme.primary,
        linearTrackColor: scheme.surfaceContainerHighest,
        circularTrackColor: scheme.surfaceContainerHighest,
      );

  /// Switch theme
  static SwitchThemeData _switchTheme(ColorScheme scheme) => SwitchThemeData(
        thumbColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return scheme.onPrimary;
          }
          return scheme.outline;
        }),
        trackColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return scheme.primary;
          }
          return scheme.surfaceContainerHighest;
        }),
      );

  /// Chip theme
  static ChipThemeData _chipTheme(ColorScheme scheme) => ChipThemeData(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        backgroundColor: scheme.surfaceContainerLow,
        selectedColor: scheme.secondaryContainer,
      );

  /// Divider theme
  static DividerThemeData _dividerTheme(ColorScheme scheme) => DividerThemeData(
        color: scheme.outlineVariant,
        thickness: 1,
        space: 1,
      );
}
