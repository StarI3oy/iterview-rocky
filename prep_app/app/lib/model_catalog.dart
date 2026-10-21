import 'models.dart';

bool? asBoolOpt(dynamic v) {
  if (v is bool) return v;
  return null;
}

double? asDoubleOpt(dynamic v) {
  if (v is num) return v.toDouble();
  return null;
}

class ModelSettings {
  const ModelSettings({
    required this.ai,
    required this.model,
    required this.provider,
    required this.providerName,
    required this.source,
    required this.envModel,
    required this.latencyNeedsKey,
  });

  final bool ai;
  final String model;
  final String provider;
  final String providerName;
  final String source;
  final String envModel;
  final bool latencyNeedsKey;

  String get providerLabel {
    if (provider.isEmpty) return 'автовыбор OpenRouter';
    if (providerName.isEmpty) return provider;
    return providerName;
  }

  String get sourceLabel =>
      source == 'settings' ? 'выбрано в настройках' : 'из OPENROUTER_MODEL';

  factory ModelSettings.fromJson(Map<String, dynamic> j) {
    return ModelSettings(
      ai: asBool(j['ai']),
      model: (j['model'] ?? '').toString(),
      provider: (j['provider'] ?? '').toString(),
      providerName: (j['provider_name'] ?? '').toString(),
      source: (j['source'] ?? '').toString(),
      envModel: (j['env_model'] ?? '').toString(),
      latencyNeedsKey: asBool(j['latency_needs_key']),
    );
  }
}

class CatalogPage {
  const CatalogPage({
    required this.query,
    required this.matchCount,
    required this.truncated,
    required this.latencyNeedsKey,
    required this.dataPolicyLoaded,
    required this.results,
  });

  final String query;
  final int matchCount;
  final bool truncated;
  final bool latencyNeedsKey;
  final bool dataPolicyLoaded;
  final List<CatalogModel> results;

  factory CatalogPage.fromJson(Map<String, dynamic> j) {
    final raw = j['results'];
    final results = raw is List
        ? raw.map((e) => CatalogModel.fromJson(asMap(e))).toList()
        : const <CatalogModel>[];
    return CatalogPage(
      query: (j['query'] ?? '').toString(),
      matchCount: asInt(j['match_count']),
      truncated: asBool(j['truncated']),
      latencyNeedsKey: asBool(j['latency_needs_key']),
      dataPolicyLoaded: asBool(j['data_policy_loaded']),
      results: results,
    );
  }
}

class CatalogModel {
  const CatalogModel({
    required this.id,
    required this.name,
    required this.description,
    required this.contextLength,
    required this.intelligenceIndex,
    required this.codingIndex,
    required this.agenticIndex,
    required this.benchmarkLines,
    required this.benchmarkNote,
    required this.priceHint,
    required this.pricingLines,
    required this.overrideLines,
    required this.providers,
    required this.providersError,
  });

  final String id;
  final String name;
  final String description;
  final int contextLength;
  final double? intelligenceIndex;
  final double? codingIndex;
  final double? agenticIndex;
  final List<String> benchmarkLines;
  final String benchmarkNote;
  final String priceHint;
  final List<String> pricingLines;
  final List<String> overrideLines;
  final List<CatalogProvider> providers;
  final String providersError;

  factory CatalogModel.fromJson(Map<String, dynamic> j) {
    final raw = j['providers'];
    return CatalogModel(
      id: (j['id'] ?? '').toString(),
      name: (j['name'] ?? '').toString(),
      description: (j['description'] ?? '').toString(),
      contextLength: asInt(j['context_length']),
      intelligenceIndex: asDoubleOpt(j['intelligence_index']),
      codingIndex: asDoubleOpt(j['coding_index']),
      agenticIndex: asDoubleOpt(j['agentic_index']),
      benchmarkLines: asStrings(j['benchmark_lines']),
      benchmarkNote: (j['benchmark_note'] ?? '').toString(),
      priceHint: (j['price_hint'] ?? '').toString(),
      pricingLines: asStrings(j['pricing_lines']),
      overrideLines: asStrings(j['override_lines']),
      providers: raw is List
          ? raw.map((e) => CatalogProvider.fromJson(asMap(e))).toList()
          : const [],
      providersError: (j['providers_error'] ?? '').toString(),
    );
  }
}

class CatalogProvider {
  const CatalogProvider({
    required this.name,
    required this.tag,
    required this.quantization,
    required this.serviceTier,
    required this.contextLength,
    required this.maxCompletionTokens,
    required this.availability,
    required this.uptimeLine,
    required this.latencyLine,
    required this.throughputLine,
    required this.collectsData,
    required this.dataLine,
    required this.pricingLines,
    required this.overrideLines,
  });

  final String name;
  final String tag;
  final String quantization;
  final String serviceTier;
  final int contextLength;
  final int? maxCompletionTokens;
  final String availability;
  final String uptimeLine;
  final String latencyLine;
  final String throughputLine;
  final bool? collectsData;
  final String dataLine;
  final List<String> pricingLines;
  final List<String> overrideLines;

  factory CatalogProvider.fromJson(Map<String, dynamic> j) {
    return CatalogProvider(
      name: (j['name'] ?? '').toString(),
      tag: (j['tag'] ?? '').toString(),
      quantization: (j['quantization'] ?? '').toString(),
      serviceTier: (j['service_tier'] ?? '').toString(),
      contextLength: asInt(j['context_length']),
      maxCompletionTokens: asIntOpt(j['max_completion_tokens']),
      availability: (j['availability'] ?? '').toString(),
      uptimeLine: (j['uptime_line'] ?? '').toString(),
      latencyLine: (j['latency_line'] ?? '').toString(),
      throughputLine: (j['throughput_line'] ?? '').toString(),
      collectsData: asBoolOpt(j['collects_data']),
      dataLine: (j['data_line'] ?? '').toString(),
      pricingLines: asStrings(j['pricing_lines']),
      overrideLines: asStrings(j['override_lines']),
    );
  }
}
