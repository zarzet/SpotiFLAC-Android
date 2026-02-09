import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

const double kNormalizedHeaderTopPadding = 24.0;

double normalizedHeaderTopPadding(
  BuildContext context, {
  double max = kNormalizedHeaderTopPadding,
}) {
  if (defaultTargetPlatform == TargetPlatform.iOS ||
      defaultTargetPlatform == TargetPlatform.macOS) {
    return 0;
  }
  final topPadding = MediaQuery.paddingOf(context).top;
  if (topPadding <= 0) return 0;
  return topPadding > max ? max : topPadding;
}
