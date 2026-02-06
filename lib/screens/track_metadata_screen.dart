import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:spotiflac_android/services/cover_cache_manager.dart';
import 'package:spotiflac_android/services/library_database.dart';
import 'package:spotiflac_android/services/palette_service.dart';
import 'package:spotiflac_android/utils/file_access.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:share_plus/share_plus.dart';
import 'package:spotiflac_android/providers/download_queue_provider.dart';
import 'package:spotiflac_android/services/platform_bridge.dart';
import 'package:spotiflac_android/l10n/l10n.dart';

class TrackMetadataScreen extends ConsumerStatefulWidget {
  final DownloadHistoryItem? item;
  final LocalLibraryItem? localItem;

  const TrackMetadataScreen({super.key, this.item, this.localItem})
      : assert(item != null || localItem != null, 'Either item or localItem must be provided');

  @override
  ConsumerState<TrackMetadataScreen> createState() => _TrackMetadataScreenState();
}

class _TrackMetadataScreenState extends ConsumerState<TrackMetadataScreen> {
  bool _fileExists = false;
  int? _fileSize;
  String? _lyrics;        // Cleaned lyrics for display (no timestamps)
  String? _rawLyrics;     // Raw LRC with timestamps for embedding
  bool _lyricsLoading = false;
  String? _lyricsError;
  Color? _dominantColor;
  bool _showTitleInAppBar = false;
  bool _lyricsEmbedded = false;  // Track if lyrics are embedded in file
  bool _isEmbedding = false;     // Track embed operation in progress
  bool _isInstrumental = false;  // Track if detected as instrumental
  final ScrollController _scrollController = ScrollController();
  static final RegExp _lrcTimestampPattern =
      RegExp(r'^\[\d{2}:\d{2}\.\d{2,3}\]');
  static final RegExp _lrcMetadataPattern =
      RegExp(r'^\[[a-zA-Z]+:.*\]$');
  static const List<String> _months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];

  String? _normalizeOptionalString(String? value) {
    if (value == null) return null;
    final trimmed = value.trim();
    if (trimmed.isEmpty) return null;
    if (trimmed.toLowerCase() == 'null') return null;
    return trimmed;
  }

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
    _checkFile();
    // Delay palette extraction to avoid jitter during initial build
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _extractDominantColor();
    });
  }

  @override
  void dispose() {
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    super.dispose();
  }

  void _onScroll() {
    final shouldShow = _scrollController.offset > 280;
    if (shouldShow != _showTitleInAppBar) {
      setState(() => _showTitleInAppBar = shouldShow);
    }
  }

  Future<void> _extractDominantColor() async {
    // For local items with cover path, extract from file
    if (_isLocalItem && _localCoverPath != null && _localCoverPath!.isNotEmpty) {
      final color = await PaletteService.instance.extractDominantColorFromFile(_localCoverPath!);
      if (mounted && color != null && color != _dominantColor) {
        setState(() => _dominantColor = color);
      }
      return;
    }
    
    final coverUrl = _coverUrl;
    if (coverUrl == null) return;
    
    // Check cache first
    final cachedColor = PaletteService.instance.getCached(coverUrl);
    if (cachedColor != null) {
      if (mounted && cachedColor != _dominantColor) {
        setState(() => _dominantColor = cachedColor);
      }
      return;
    }
    
    // Extract using PaletteService (runs in isolate)
    final color = await PaletteService.instance.extractDominantColor(coverUrl);
    if (mounted && color != null && color != _dominantColor) {
      setState(() => _dominantColor = color);
    }
  }

  Future<void> _checkFile() async {
    var filePath = _filePath;
    if (filePath.startsWith('EXISTS:')) {
      filePath = filePath.substring(7);
    }

    bool exists = false;
    int? size;
    try {
      final stat = await fileStat(filePath);
      if (stat != null) {
        exists = true;
        size = stat.size;
      }
    } catch (_) {}

    if (mounted && (exists != _fileExists || size != _fileSize)) {
      setState(() {
        _fileExists = exists;
        _fileSize = size;
      });
    }

    if (mounted && exists && _lyrics == null && !_lyricsLoading) {
      _fetchLyrics();
    }
  }

  bool get _isLocalItem => widget.localItem != null;
  DownloadHistoryItem? get _downloadItem => widget.item;
  LocalLibraryItem? get _localLibraryItem => widget.localItem;
  
  String get _itemId => _isLocalItem ? _localLibraryItem!.id : _downloadItem!.id;
  String get trackName => _isLocalItem ? _localLibraryItem!.trackName : _downloadItem!.trackName;
  String get artistName => _isLocalItem ? _localLibraryItem!.artistName : _downloadItem!.artistName;
  String get albumName => _isLocalItem ? _localLibraryItem!.albumName : _downloadItem!.albumName;
  String? get albumArtist => _normalizeOptionalString(_isLocalItem ? _localLibraryItem!.albumArtist : _downloadItem!.albumArtist);
  int? get trackNumber => _isLocalItem ? _localLibraryItem!.trackNumber : _downloadItem!.trackNumber;
  int? get discNumber => _isLocalItem ? _localLibraryItem!.discNumber : _downloadItem!.discNumber;
  String? get releaseDate => _isLocalItem ? _localLibraryItem!.releaseDate : _downloadItem!.releaseDate;
  String? get isrc => _isLocalItem ? _localLibraryItem!.isrc : _downloadItem!.isrc;
  String? get genre => _isLocalItem ? _localLibraryItem!.genre : _downloadItem!.genre;
  String? get label => _isLocalItem ? null : _downloadItem!.label;
  String? get copyright => _isLocalItem ? null : _downloadItem!.copyright;
  int? get duration => _isLocalItem ? _localLibraryItem!.duration : _downloadItem!.duration;
  int? get bitDepth => _isLocalItem ? _localLibraryItem!.bitDepth : _downloadItem!.bitDepth;
  int? get sampleRate => _isLocalItem ? _localLibraryItem!.sampleRate : _downloadItem!.sampleRate;
  
  String get _filePath => _isLocalItem ? _localLibraryItem!.filePath : _downloadItem!.filePath;
  String? get _coverUrl => _isLocalItem ? null : _downloadItem!.coverUrl;
  String? get _localCoverPath => _isLocalItem ? _localLibraryItem!.coverPath : null;
  String? get _spotifyId => _isLocalItem ? null : _downloadItem!.spotifyId;
  String get _service => _isLocalItem ? 'local' : _downloadItem!.service;
  DateTime get _addedAt => _isLocalItem ? _localLibraryItem!.scannedAt : _downloadItem!.downloadedAt;
  String? get _quality => _isLocalItem ? null : _downloadItem!.quality;
  
  String get cleanFilePath {
    final path = _filePath;
    return path.startsWith('EXISTS:') ? path.substring(7) : path;
  }

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final screenWidth = MediaQuery.of(context).size.width;
    final coverSize = screenWidth * 0.5;
    final bgColor = _dominantColor ?? colorScheme.surface;

    return Scaffold(
      body: CustomScrollView(
        controller: _scrollController,
        slivers: [
          SliverAppBar(
            expandedHeight: 320,
            pinned: true,
            stretch: true,
            backgroundColor: colorScheme.surface, // Use theme color for collapsed state
            surfaceTintColor: Colors.transparent,
            title: AnimatedOpacity(
              duration: const Duration(milliseconds: 200),
              opacity: _showTitleInAppBar ? 1.0 : 0.0,
              child: Text(
                trackName,
                style: TextStyle(
                  color: colorScheme.onSurface,
                  fontWeight: FontWeight.w600,
                  fontSize: 16,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            flexibleSpace: LayoutBuilder(
              builder: (context, constraints) {
                final collapseRatio = (constraints.maxHeight - kToolbarHeight) / (320 - kToolbarHeight);
                final showContent = collapseRatio > 0.3;
                
                return FlexibleSpaceBar(
                  collapseMode: CollapseMode.none,
                  background: _buildHeaderBackground(context, colorScheme, coverSize, bgColor, showContent),
                  stretchModes: const [
                    StretchMode.zoomBackground,
                    StretchMode.blurBackground,
                  ],
                );
              },
            ),
            leading: IconButton(
              icon: Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: colorScheme.surface.withValues(alpha: 0.8),
                  shape: BoxShape.circle,
                ),
                child: Icon(Icons.arrow_back, color: colorScheme.onSurface),
              ),
              onPressed: () => Navigator.pop(context),
            ),
            actions: [
              IconButton(
                icon: Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: colorScheme.surface.withValues(alpha: 0.8),
                    shape: BoxShape.circle,
                  ),
                  child: Icon(Icons.more_vert, color: colorScheme.onSurface),
                ),
                onPressed: () => _showOptionsMenu(context, ref, colorScheme),
              ),
            ],
          ),

          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildTrackInfoCard(context, colorScheme, _fileExists),
                  
                  const SizedBox(height: 16),
                  
                  _buildMetadataCard(context, colorScheme, _fileSize),
                  
                  const SizedBox(height: 16),
                  
                  _buildFileInfoCard(context, colorScheme, _fileExists, _fileSize),
                  
                  const SizedBox(height: 16),
                  
                  _buildLyricsCard(context, colorScheme),
                  
                  const SizedBox(height: 24),
                  
                  _buildActionButtons(context, ref, colorScheme, _fileExists),
                  
                  const SizedBox(height: 32),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildHeaderBackground(BuildContext context, ColorScheme colorScheme, double coverSize, Color bgColor, bool showContent) {
    return Stack(
      fit: StackFit.expand,
      children: [
        AnimatedContainer(
          duration: const Duration(milliseconds: 500),
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [
                bgColor,
                bgColor.withValues(alpha: 0.8),
                colorScheme.surface,
              ],
              stops: const [0.0, 0.6, 1.0],
            ),
          ),
        ),
        
        AnimatedOpacity(
          duration: const Duration(milliseconds: 150),
          opacity: showContent ? 1.0 : 0.0,
          child: Center(
            child: Padding(
              padding: const EdgeInsets.only(top: 60),
              child: Hero(
                tag: 'cover_$_itemId',
                child: Container(
                  width: coverSize,
                  height: coverSize,
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(20),
                    boxShadow: [
                      BoxShadow(
                        color: Colors.black.withValues(alpha: 0.4),
                        blurRadius: 30,
                        offset: const Offset(0, 15),
                      ),
                    ],
                  ),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(20),
                    child: _coverUrl != null
                        ? CachedNetworkImage(
                            imageUrl: _coverUrl!,
                            fit: BoxFit.cover,
                            memCacheWidth: (coverSize * 2).toInt(),
                            cacheManager: CoverCacheManager.instance,
                            placeholder: (_, _) => Container(
                              color: colorScheme.surfaceContainerHighest,
                              child: Icon(
                                Icons.music_note,
                                size: 64,
                                color: colorScheme.onSurfaceVariant,
                              ),
                            ),
                          )
                        : _localCoverPath != null && _localCoverPath!.isNotEmpty
                            ? Image.file(
                                File(_localCoverPath!),
                                fit: BoxFit.cover,
                              )
                            : Container(
                                color: colorScheme.surfaceContainerHighest,
                                child: Icon(
                                  Icons.music_note,
                                  size: 64,
                                  color: colorScheme.onSurfaceVariant,
                                ),
                              ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildTrackInfoCard(BuildContext context, ColorScheme colorScheme, bool fileExists) {
    return Card(
      elevation: 0,
      color: colorScheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              trackName,
              style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                fontWeight: FontWeight.bold,
                color: colorScheme.onSurface,
              ),
            ),
            const SizedBox(height: 4),
            
            Text(
              artistName,
              style: Theme.of(context).textTheme.titleMedium?.copyWith(
                color: colorScheme.primary,
              ),
            ),
            const SizedBox(height: 8),
            
            Row(
              children: [
                Icon(
                  Icons.album,
                  size: 16,
                  color: colorScheme.onSurfaceVariant,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    albumName,
                    style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
              ],
            ),
            
            if (!fileExists) ...[
              const SizedBox(height: 12),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                decoration: BoxDecoration(
                  color: colorScheme.errorContainer,
                  borderRadius: BorderRadius.circular(20),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      Icons.warning_rounded,
                      size: 16,
                      color: colorScheme.onErrorContainer,
                    ),
                    const SizedBox(width: 6),
                    Text(
                      context.l10n.trackFileNotFound,
                      style: TextStyle(
                        color: colorScheme.onErrorContainer,
                        fontSize: 12,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildMetadataCard(BuildContext context, ColorScheme colorScheme, int? fileSize) {
    return Card(
      elevation: 0,
      color: colorScheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.info_outline,
                  size: 20,
                  color: colorScheme.primary,
                ),
                const SizedBox(width: 8),
                Text(
                  context.l10n.trackMetadata,
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: colorScheme.onSurface,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            
            _buildMetadataGrid(context, colorScheme),
            
            if (_spotifyId != null && _spotifyId!.isNotEmpty) ...[
              const SizedBox(height: 8),
              Builder(
                builder: (context) {
                  final isDeezer = _spotifyId!.contains('deezer');
                  return OutlinedButton.icon(
                    onPressed: () => _openServiceUrl(context),
                    icon: const Icon(Icons.open_in_new, size: 18),
                    label: Text(isDeezer ? context.l10n.trackOpenInDeezer : context.l10n.trackOpenInSpotify),
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(12),
                      ),
                    ),
                  );
                }
              ),
            ],
          ],
        ),
      ),
    );
  }

  Future<void> _openServiceUrl(BuildContext context) async {
    if (_spotifyId == null) return;
    
    final isDeezer = _spotifyId!.contains('deezer');
    final rawId = _spotifyId!.replaceAll('deezer:', '');
    
    final webUrl = isDeezer 
        ? 'https://www.deezer.com/track/$rawId'
        : 'https://open.spotify.com/track/$rawId';
        
    final appUri = isDeezer
        ? Uri.parse('deezer://www.deezer.com/track/$rawId')
        : Uri.parse('spotify:track:$rawId');
    
    try {
      final launched = await launchUrl(
        appUri,
        mode: LaunchMode.externalApplication,
      );
      
      if (!launched) {
        await launchUrl(
          Uri.parse(webUrl),
          mode: LaunchMode.externalApplication,
        );
      }
    } catch (e) {
      try {
        await launchUrl(
          Uri.parse(webUrl),
          mode: LaunchMode.externalApplication,
        );
      } catch (_) {
        if (context.mounted) {
          _copyToClipboard(context, webUrl);
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(context.l10n.snackbarUrlCopied(isDeezer ? 'Deezer' : 'Spotify'))),
          );
        }
      }
    }
  }

  Widget _buildMetadataGrid(BuildContext context, ColorScheme colorScheme) {
    // Determine audio quality string - prefer stored quality from download
    String? audioQualityStr;
    final fileName = _filePath.split('/').last;
    final fileExt = fileName.contains('.') ? fileName.split('.').last.toUpperCase() : '';
    
    // Use stored quality from download history if available
    if (_quality != null && _quality!.isNotEmpty) {
      audioQualityStr = _quality;
    } else if (bitDepth != null && sampleRate != null) {
      // Fallback for FLAC files without stored quality
      final sampleRateKHz = (sampleRate! / 1000).toStringAsFixed(1);
      audioQualityStr = '$bitDepth-bit/${sampleRateKHz}kHz';
    } else {
      // Fallback based on file extension for legacy items
      if (fileExt == 'MP3') {
        audioQualityStr = 'MP3';
      } else if (fileExt == 'OPUS' || fileExt == 'OGG') {
        audioQualityStr = 'Opus';
      } else if (fileExt == 'M4A' || fileExt == 'AAC') {
        audioQualityStr = 'AAC';
      }
    }
    
    final items = <_MetadataItem>[
      _MetadataItem(context.l10n.trackTrackName, trackName),
      _MetadataItem(context.l10n.trackArtist, artistName),
      if (albumArtist != null && albumArtist != artistName)
        _MetadataItem(context.l10n.trackAlbumArtist, albumArtist!),
      _MetadataItem(context.l10n.trackAlbum, albumName),
      if (trackNumber != null && trackNumber! > 0)
        _MetadataItem(context.l10n.trackTrackNumber, trackNumber.toString()),
      if (discNumber != null && discNumber! > 0)
        _MetadataItem(context.l10n.trackDiscNumber, discNumber.toString()),
      if (duration != null)
        _MetadataItem(context.l10n.trackDuration, _formatDuration(duration!)),
      if (audioQualityStr != null)
        _MetadataItem(context.l10n.trackAudioQuality, audioQualityStr),
      if (releaseDate != null && releaseDate!.isNotEmpty)
        _MetadataItem(context.l10n.trackReleaseDate, releaseDate!),
      if (genre != null && genre!.isNotEmpty)
        _MetadataItem(context.l10n.trackGenre, genre!),
      if (label != null && label!.isNotEmpty)
        _MetadataItem(context.l10n.trackLabel, label!),
      if (copyright != null && copyright!.isNotEmpty)
        _MetadataItem(context.l10n.trackCopyright, copyright!),
      if (isrc != null && isrc!.isNotEmpty)
        _MetadataItem('ISRC', isrc!),
    ];
    
    if (!_isLocalItem && _spotifyId != null && _spotifyId!.isNotEmpty) {
      final isDeezer = _spotifyId!.contains('deezer');
      final cleanId = _spotifyId!.replaceAll('deezer:', '');
      items.add(_MetadataItem(isDeezer ? 'Deezer ID' : 'Spotify ID', cleanId));
    }
    
    items.add(_MetadataItem(context.l10n.trackMetadataService, _service.toUpperCase()));
    items.add(_MetadataItem(
      context.l10n.trackDownloaded,
      _formatFullDate(_addedAt),
    ));

    return Column(
      children: items.map((metadata) {
        final isCopyable = metadata.label == 'ISRC' || 
                          metadata.label == 'Spotify ID';
        return InkWell(
          onTap: isCopyable ? () => _copyToClipboard(context, metadata.value) : null,
          borderRadius: BorderRadius.circular(8),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 6, horizontal: 4),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SizedBox(
                  width: 100,
                  child: Text(
                    metadata.label,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
                Expanded(
                  child: Text(
                    metadata.value,
                    style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                      color: colorScheme.onSurface,
                    ),
                  ),
                ),
                if (isCopyable)
                  Icon(
                    Icons.copy,
                    size: 14,
                    color: colorScheme.onSurfaceVariant.withValues(alpha: 0.5),
                  ),
              ],
            ),
          ),
        );
      }).toList(),
    );
  }

  String _formatDuration(int seconds) {
    final minutes = seconds ~/ 60;
    final secs = seconds % 60;
    return '$minutes:${secs.toString().padLeft(2, '0')}';
  }

  Widget _buildFileInfoCard(BuildContext context, ColorScheme colorScheme, bool fileExists, int? fileSize) {
    final fileName = cleanFilePath.split(Platform.pathSeparator).last;
    final fileExtension = fileName.contains('.') ? fileName.split('.').last.toUpperCase() : 'Unknown';
    
    return Card(
      elevation: 0,
      color: colorScheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.folder_outlined,
                  size: 20,
                  color: colorScheme.primary,
                ),
                const SizedBox(width: 8),
                Text(
                  context.l10n.trackFileInfo,
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: colorScheme.onSurface,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(
                    color: colorScheme.primaryContainer,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Text(
                    fileExtension,
                    style: TextStyle(
                      color: colorScheme.onPrimaryContainer,
                      fontWeight: FontWeight.w600,
                      fontSize: 12,
                    ),
                  ),
                ),
                if (fileSize != null)
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    decoration: BoxDecoration(
                      color: colorScheme.secondaryContainer,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Text(
                      _formatFileSize(fileSize),
                      style: TextStyle(
                        color: colorScheme.onSecondaryContainer,
                        fontWeight: FontWeight.w600,
                        fontSize: 12,
                      ),
                    ),
                  ),
                if (fileExtension == 'MP3')
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    decoration: BoxDecoration(
                      color: colorScheme.tertiaryContainer,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Text(
                      '320kbps',
                      style: TextStyle(
                        color: colorScheme.onTertiaryContainer,
                        fontWeight: FontWeight.w600,
                        fontSize: 12,
                      ),
                    ),
                  )
                else if (bitDepth != null && sampleRate != null)
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    decoration: BoxDecoration(
                      color: colorScheme.tertiaryContainer,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Text(
                      '$bitDepth-bit/${(sampleRate! / 1000).toStringAsFixed(1)}kHz',
                      style: TextStyle(
                        color: colorScheme.onTertiaryContainer,
                        fontWeight: FontWeight.w600,
                        fontSize: 12,
                      ),
                    ),
                  ),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(
                    color: _getServiceColor(_service, colorScheme),
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        _getServiceIcon(_service),
                        size: 14,
                        color: Colors.white,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        _service.toUpperCase(),
                        style: const TextStyle(
                          color: Colors.white,
                          fontWeight: FontWeight.w600,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            
            const SizedBox(height: 16),
            
            InkWell(
              onTap: () => _copyToClipboard(context, cleanFilePath),
              borderRadius: BorderRadius.circular(12),
              child: Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: colorScheme.surfaceContainerHighest,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        cleanFilePath,
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          fontFamily: 'monospace',
                          color: colorScheme.onSurfaceVariant,
                        ),
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Icon(
                      Icons.copy,
                      size: 18,
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLyricsCard(BuildContext context, ColorScheme colorScheme) {
    return Card(
      elevation: 0,
      color: colorScheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.lyrics_outlined,
                  size: 20,
                  color: colorScheme.primary,
                ),
                const SizedBox(width: 8),
                Text(
                  context.l10n.trackLyrics,
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: colorScheme.onSurface,
                  ),
                ),
                const Spacer(),
                if (_lyrics != null)
                  IconButton(
                    icon: const Icon(Icons.copy, size: 20),
                    onPressed: () => _copyToClipboard(context, _lyrics!),
                    tooltip: context.l10n.trackCopyLyrics,
                  ),
              ],
            ),
            const SizedBox(height: 12),
            
            if (_lyricsLoading)
              const Center(
                child: Padding(
                  padding: EdgeInsets.all(20),
                  child: CircularProgressIndicator(),
                ),
              )
            else if (_lyricsError != null)
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: colorScheme.errorContainer.withValues(alpha: 0.3),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    Icon(Icons.error_outline, color: colorScheme.error, size: 20),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        _lyricsError!,
                        style: TextStyle(color: colorScheme.onErrorContainer),
                      ),
                    ),
                    TextButton(
                      onPressed: _fetchLyrics,
                      child: Text(context.l10n.dialogRetry),
                    ),
                  ],
                ),
              )
            else if (_isInstrumental)
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: colorScheme.tertiaryContainer.withValues(alpha: 0.3),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(Icons.music_note, color: colorScheme.tertiary, size: 20),
                    const SizedBox(width: 12),
                    Text(
                      context.l10n.trackInstrumental,
                      style: TextStyle(
                        color: colorScheme.onTertiaryContainer,
                        fontStyle: FontStyle.italic,
                      ),
                    ),
                  ],
                ),
              )
            else if (_lyrics != null)
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    constraints: const BoxConstraints(maxHeight: 300),
                    child: SingleChildScrollView(
                      child: Text(
                        _lyrics!,
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          color: colorScheme.onSurface,
                          height: 1.6,
                        ),
                      ),
                    ),
                  ),
                  // Show "Embed Lyrics" button if lyrics are from online (not already embedded)
                  if (!_lyricsEmbedded && _fileExists) ...[
                    const SizedBox(height: 16),
                    Center(
                      child: FilledButton.tonalIcon(
                        onPressed: _isEmbedding ? null : _embedLyrics,
                        icon: _isEmbedding
                            ? const SizedBox(
                                width: 18,
                                height: 18,
                                child: CircularProgressIndicator(strokeWidth: 2),
                              )
                            : const Icon(Icons.save_alt),
                        label: Text(context.l10n.trackEmbedLyrics),
                      ),
                    ),
                  ],
                ],
              )
            else
              Center(
                child: FilledButton.tonalIcon(
                  onPressed: _fetchLyrics,
                  icon: const Icon(Icons.download),
                  label: Text(context.l10n.trackLoadLyrics),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _fetchLyrics() async {
    if (_lyricsLoading) return;
    
    setState(() {
      _lyricsLoading = true;
      _lyricsError = null;
      _isInstrumental = false;
    });

    try {
      // Convert duration from seconds to milliseconds
      final durationMs = (duration ?? 0) * 1000;
      
      // First, check if lyrics are embedded in the file
      if (_fileExists) {
        final embeddedResult = await PlatformBridge.getLyricsLRC(
          '',
          trackName,
          artistName,
          filePath: cleanFilePath,
          durationMs: 0,
        ).timeout(const Duration(seconds: 5), onTimeout: () => '');
        
        if (embeddedResult.isNotEmpty) {
          // Lyrics found in file
          if (mounted) {
            final cleanLyrics = _cleanLrcForDisplay(embeddedResult);
            setState(() {
              _lyrics = cleanLyrics;
              _lyricsEmbedded = true;
              _lyricsLoading = false;
            });
          }
          return;
        }
      }
      
      // No embedded lyrics, fetch from online
      final result = await PlatformBridge.getLyricsLRC(
        _spotifyId ?? '',
        trackName,
        artistName,
        filePath: null, // Don't check file again
        durationMs: durationMs,
      ).timeout(
        const Duration(seconds: 20),
        onTimeout: () => '',
      );
      
      if (mounted) {
        // Check for instrumental marker
        if (result == '[instrumental:true]') {
          setState(() {
            _isInstrumental = true;
            _lyricsLoading = false;
          });
        } else if (result.isEmpty) {
          setState(() {
            _lyricsError = context.l10n.trackLyricsNotAvailable;
            _lyricsLoading = false;
          });
        } else {
          final cleanLyrics = _cleanLrcForDisplay(result);
          setState(() {
            _lyrics = cleanLyrics;
            _rawLyrics = result; // Keep raw LRC with timestamps for embedding
            _lyricsEmbedded = false; // Lyrics from online, not embedded
            _lyricsLoading = false;
          });
        }
      }
    } catch (e) {
      if (mounted) {
        final errorMsg = e.toString().contains('TimeoutException') 
            ? context.l10n.trackLyricsTimeout
            : context.l10n.trackLyricsLoadFailed;
        setState(() {
          _lyricsError = errorMsg;
          _lyricsLoading = false;
        });
      }
    }
  }
  
  Future<void> _embedLyrics() async {
    if (_isEmbedding || _rawLyrics == null || !_fileExists) return;
    
    setState(() => _isEmbedding = true);
    
    try {
      // Use raw LRC content directly - it already has timestamps and metadata
      final result = await PlatformBridge.embedLyricsToFile(
        cleanFilePath,
        _rawLyrics!,
      );
      
      if (mounted) {
        if (result['success'] == true) {
          setState(() {
            _lyricsEmbedded = true;
            _isEmbedding = false;
          });
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(context.l10n.trackLyricsEmbedded)),
          );
        } else {
          setState(() => _isEmbedding = false);
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(result['error'] ?? 'Failed to embed lyrics')),
          );
        }
      }
    } catch (e) {
      if (mounted) {
        setState(() => _isEmbedding = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Error: $e')),
        );
      }
    }
  }

  String _cleanLrcForDisplay(String lrc) {
    final lines = lrc.split('\n');
    final cleanLines = <String>[];
    
    for (final line in lines) {
      final trimmedLine = line.trim();
      
      // Skip metadata tags
      if (_lrcMetadataPattern.hasMatch(trimmedLine)) {
        continue;
      }
      
      // Remove timestamp and clean up
      final cleanLine = trimmedLine.replaceAll(_lrcTimestampPattern, '').trim();
      if (cleanLine.isNotEmpty) {
        cleanLines.add(cleanLine);
      }
    }
    
    return cleanLines.join('\n');
  }

  Widget _buildActionButtons(BuildContext context, WidgetRef ref, ColorScheme colorScheme, bool fileExists) {
    return Row(
      children: [
        Expanded(
          flex: 2,
          child: FilledButton.icon(
            onPressed: fileExists ? () => _openFile(context, cleanFilePath) : null,
            icon: const Icon(Icons.play_arrow),
            label: Text(context.l10n.trackMetadataPlay),
            style: FilledButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(16),
              ),
            ),
          ),
        ),
        const SizedBox(width: 12),
        
        Expanded(
          child: OutlinedButton.icon(
            onPressed: () => _confirmDelete(context, ref, colorScheme),
            icon: Icon(Icons.delete_outline, color: colorScheme.error),
            label: Text(context.l10n.trackMetadataDelete, style: TextStyle(color: colorScheme.error)),
            style: OutlinedButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(16),
              ),
              side: BorderSide(color: colorScheme.error.withValues(alpha: 0.5)),
            ),
          ),
        ),
      ],
    );
  }

  void _showOptionsMenu(BuildContext context, WidgetRef ref, ColorScheme colorScheme) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(height: 8),
            Container(
              width: 40,
              height: 4,
              decoration: BoxDecoration(
                color: colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const SizedBox(height: 16),
            ListTile(
              leading: const Icon(Icons.copy),
              title: Text(context.l10n.trackCopyFilePath),
              onTap: () {
                Navigator.pop(context);
                _copyToClipboard(context, cleanFilePath);
              },
            ),
            ListTile(
              leading: const Icon(Icons.share),
              title: Text(context.l10n.trackMetadataShare),
              onTap: () {
                Navigator.pop(context);
                _shareFile(context);
              },
            ),
            ListTile(
              leading: Icon(Icons.delete, color: colorScheme.error),
              title: Text(context.l10n.trackRemoveFromDevice, style: TextStyle(color: colorScheme.error)),
              onTap: () {
                Navigator.pop(context);
                _confirmDelete(context, ref, colorScheme);
              },
            ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    );
  }

  void _confirmDelete(BuildContext context, WidgetRef ref, ColorScheme colorScheme) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(context.l10n.trackDeleteConfirmTitle),
        content: Text(context.l10n.trackDeleteConfirmMessage),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(context.l10n.dialogCancel),
          ),
          TextButton(
            onPressed: () async {
              if (_isLocalItem) {
                // For local items, just delete the file
                try {
                  await deleteFile(cleanFilePath);
                } catch (e) {
                  debugPrint('Failed to delete file: $e');
                }
                // Also remove from local library database
                // ref.read(localLibraryProvider.notifier).removeItem(_localLibraryItem!.id);
              } else {
                // Existing download history deletion logic
                try {
                  await deleteFile(cleanFilePath);
                } catch (e) {
                  debugPrint('Failed to delete file: $e');
                }
                
                ref.read(downloadHistoryProvider.notifier).removeFromHistory(_downloadItem!.id);
              }
              
              if (context.mounted) {
                Navigator.pop(context);
                Navigator.pop(context);
              }
            },
            child: Text(context.l10n.dialogDelete, style: TextStyle(color: colorScheme.error)),
          ),
        ],
      ),
    );
  }

  Future<void> _openFile(BuildContext context, String filePath) async {
    try {
      await openFile(filePath);
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.snackbarCannotOpenFile(e.toString()))),
        );
      }
    }
  }

  void _copyToClipboard(BuildContext context, String text) {
    Clipboard.setData(ClipboardData(text: text));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(context.l10n.trackCopiedToClipboard),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  Future<void> _shareFile(BuildContext context) async {
    String sharePath = cleanFilePath;
    if (!await fileExists(sharePath)) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.snackbarFileNotFound)),
        );
      }
      return;
    }

    if (isContentUri(sharePath)) {
      final tempPath = await PlatformBridge.copyContentUriToTemp(sharePath);
      if (tempPath == null || tempPath.isEmpty) {
        if (context.mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(context.l10n.snackbarCannotOpenFile('Failed to prepare file for sharing'))),
          );
        }
        return;
      }
      sharePath = tempPath;
    }
    
    await SharePlus.instance.share(
      ShareParams(
        files: [XFile(sharePath)],
        text: '$trackName - $artistName',
      ),
    );
  }

  String _formatFullDate(DateTime date) {
    return '${date.day} ${_months[date.month - 1]} ${date.year}, '
           '${date.hour.toString().padLeft(2, '0')}:'
           '${date.minute.toString().padLeft(2, '0')}';
  }

  String _formatFileSize(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }

  IconData _getServiceIcon(String service) {
    switch (service.toLowerCase()) {
      case 'tidal':
        return Icons.waves;
      case 'qobuz':
        return Icons.album;
      case 'amazon':
        return Icons.shopping_cart;
      default:
        return Icons.cloud_download;
    }
  }

  Color _getServiceColor(String service, ColorScheme colorScheme) {
    switch (service.toLowerCase()) {
      case 'tidal':
        return const Color(0xFF0077B5); // Tidal blue (darker, more readable)
      case 'qobuz':
        return const Color(0xFF0052CC); // Qobuz blue
      case 'amazon':
        return const Color(0xFFFF9900); // Amazon orange
      default:
        return colorScheme.primary;
    }
  }
}

class _MetadataItem {
  final String label;
  final String value;
  
  _MetadataItem(this.label, this.value);
}
