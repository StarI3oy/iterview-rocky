class ImportStats {
  const ImportStats({
    required this.cards,
    required this.questions,
    required this.prepItems,
    required this.concepts,
    required this.tracker,
  });

  final int cards;
  final int questions;
  final int prepItems;
  final int concepts;
  final int tracker;

  factory ImportStats.fromJson(Map<String, dynamic> j) {
    return ImportStats(
      cards: asInt(j['imported']),
      questions: asInt(j['questions']),
      prepItems: asInt(j['prep_items']),
      concepts: asInt(j['concepts']),
      tracker: asInt(j['tracker']),
    );
  }
}

int asInt(dynamic v, [int fallback = 0]) {
  if (v is int) return v;
  if (v is num) return v.toInt();
  return fallback;
}

int? asIntOpt(dynamic v) {
  if (v == null) return null;
  if (v is int) return v;
  if (v is num) return v.toInt();
  return null;
}

bool asBool(dynamic v) {
  if (v is bool) return v;
  if (v is num) return v != 0;
  return false;
}

List<String> asStrings(dynamic v) {
  if (v is! List) return const [];
  return v.map((e) => e.toString()).toList();
}

Map<String, dynamic> asMap(dynamic v) {
  if (v is Map<String, dynamic>) return v;
  if (v is Map) return Map<String, dynamic>.from(v);
  return const {};
}

class PrepItem {
  const PrepItem({
    required this.id,
    required this.itemType,
    required this.block,
    required this.blockTitle,
    required this.company,
    required this.title,
    required this.prompt,
    required this.options,
    required this.correctIndex,
    required this.explanation,
    required this.answerSkeleton,
    required this.checklist,
    required this.starterCode,
    required this.reviewCriteria,
    required this.language,
    required this.timesAsked,
    required this.closed,
    required this.requiresCode,
  });

  final int id;
  final String itemType;
  final String block;
  final String blockTitle;
  final String company;
  final String title;
  final String prompt;
  final List<String> options;
  final int? correctIndex;
  final String explanation;
  final String answerSkeleton;
  final List<String> checklist;
  final String starterCode;
  final String reviewCriteria;
  final String language;
  final int timesAsked;
  final bool closed;
  final bool requiresCode;

  bool get isCode => itemType == 'code' || requiresCode;

  String get heading => title.trim().isEmpty ? prompt : title;

  factory PrepItem.fromJson(Map<String, dynamic> j) {
    return PrepItem(
      id: asInt(j['id']),
      itemType: (j['item_type'] ?? 'flashcard').toString(),
      block: (j['block'] ?? '').toString(),
      blockTitle: (j['block_title'] ?? '').toString(),
      company: (j['company'] ?? '').toString(),
      title: (j['title'] ?? '').toString(),
      prompt: (j['prompt'] ?? '').toString(),
      options: asStrings(j['options']),
      correctIndex: asIntOpt(j['correct_index']),
      explanation: (j['explanation'] ?? '').toString(),
      answerSkeleton: (j['answer_skeleton'] ?? '').toString(),
      checklist: asStrings(j['checklist']),
      starterCode: (j['starter_code'] ?? '').toString(),
      reviewCriteria: (j['review_criteria'] ?? '').toString(),
      language: (j['language'] ?? '').toString(),
      timesAsked: asInt(j['times_asked']),
      closed: asBool(j['closed']),
      requiresCode: asBool(j['requires_code']),
    );
  }

  static PrepItem? maybe(dynamic v) {
    if (v == null) return null;
    return PrepItem.fromJson(asMap(v));
  }
}

class BlockStat {
  const BlockStat({
    required this.id,
    required this.title,
    required this.percent,
    required this.done,
    required this.total,
  });

  final String id;
  final String title;
  final int percent;
  final int done;
  final int total;

  factory BlockStat.fromJson(Map<String, dynamic> j) {
    return BlockStat(
      id: (j['id'] ?? '').toString(),
      title: (j['title'] ?? '').toString(),
      percent: asInt(j['percent']),
      done: asInt(j['done']),
      total: asInt(j['total']),
    );
  }
}

class AskedQuestion {
  const AskedQuestion({
    required this.id,
    required this.topic,
    required this.question,
    required this.timesAsked,
    required this.understood,
  });

  final int id;
  final String topic;
  final String question;
  final int timesAsked;
  final bool understood;

  factory AskedQuestion.fromJson(Map<String, dynamic> j) {
    return AskedQuestion(
      id: asInt(j['id']),
      topic: (j['topic'] ?? '').toString(),
      question: (j['question'] ?? '').toString(),
      timesAsked: asInt(j['times_asked']),
      understood: asBool(j['understood']),
    );
  }
}

class BarStatus {
  const BarStatus({
    required this.overall,
    required this.done,
    required this.total,
    required this.empty,
    required this.blocks,
    this.protocol = '',
    this.today = '',
    this.todayTrack = '',
    this.studied = 0,
    this.tracker = const [],
  });

  final int overall;
  final int done;
  final int total;
  final bool empty;
  final List<BlockStat> blocks;
  final String protocol;
  final String today;
  final String todayTrack;
  final int studied;
  final List<AskedQuestion> tracker;

  factory BarStatus.fromJson(Map<String, dynamic> j) {
    final raw = j['blocks'];
    final blocks = <BlockStat>[];
    if (raw is List) {
      for (final item in raw) {
        blocks.add(BlockStat.fromJson(asMap(item)));
      }
    }
    final tracker = <AskedQuestion>[];
    if (j['tracker'] is List) {
      for (final item in j['tracker'] as List) {
        tracker.add(AskedQuestion.fromJson(asMap(item)));
      }
    }
    return BarStatus(
      overall: asInt(j['overall_percent']),
      done: asInt(j['done']),
      total: asInt(j['total']),
      empty: asBool(j['empty']),
      blocks: blocks,
      protocol: (j['protocol'] ?? '').toString(),
      today: (j['today'] ?? '').toString(),
      todayTrack: (j['today_track'] ?? '').toString(),
      studied: asInt(j['studied']),
      tracker: tracker,
    );
  }
}

class AnswerOutcome {
  const AnswerOutcome({
    required this.passed,
    required this.correctIndex,
    required this.explanation,
    required this.timesAsked,
  });

  final bool passed;
  final int? correctIndex;
  final String explanation;
  final int timesAsked;

  factory AnswerOutcome.fromJson(Map<String, dynamic> j) {
    return AnswerOutcome(
      passed: asBool(j['passed']),
      correctIndex: asIntOpt(j['correct_index']),
      explanation: (j['explanation'] ?? '').toString(),
      timesAsked: asInt(j['times_asked']),
    );
  }
}

class ReviewVerdict {
  const ReviewVerdict({
    required this.passed,
    required this.score,
    required this.bugs,
    required this.styleNotes,
    required this.followUp,
  });

  final bool passed;
  final int score;
  final List<String> bugs;
  final List<String> styleNotes;
  final String followUp;

  factory ReviewVerdict.fromJson(Map<String, dynamic> j) {
    return ReviewVerdict(
      passed: asBool(j['passed']),
      score: asInt(j['score']),
      bugs: asStrings(j['bugs']),
      styleNotes: asStrings(j['style_notes']),
      followUp: (j['follow_up_question'] ?? '').toString(),
    );
  }
}

class HardConcept {
  const HardConcept({
    required this.id,
    required this.questionId,
    required this.blockTitle,
    required this.title,
    required this.prompt,
    required this.failCount,
  });

  final int id;
  final int? questionId;
  final String blockTitle;
  final String title;
  final String prompt;
  final int failCount;

  String get heading => title.trim().isEmpty ? prompt : title;

  factory HardConcept.fromJson(Map<String, dynamic> j) {
    return HardConcept(
      id: asInt(j['id']),
      questionId: asIntOpt(j['question_id']),
      blockTitle: (j['block_title'] ?? '').toString(),
      title: (j['title'] ?? '').toString(),
      prompt: (j['prompt'] ?? '').toString(),
      failCount: asInt(j['fail_count']),
    );
  }
}

class SearchHit {
  const SearchHit({
    required this.kind,
    required this.id,
    required this.questionId,
    required this.itemType,
    required this.blockTitle,
    required this.company,
    required this.title,
    required this.snippet,
  });

  final String kind;
  final int id;
  final int? questionId;
  final String itemType;
  final String blockTitle;
  final String company;
  final String title;
  final String snippet;

  int? get openId => kind == 'item' ? id : questionId;

  factory SearchHit.fromJson(Map<String, dynamic> j) {
    return SearchHit(
      kind: (j['kind'] ?? '').toString(),
      id: asInt(j['id']),
      questionId: asIntOpt(j['question_id']),
      itemType: (j['item_type'] ?? '').toString(),
      blockTitle: (j['block_title'] ?? '').toString(),
      company: (j['company'] ?? '').toString(),
      title: (j['title'] ?? '').toString(),
      snippet: (j['snippet'] ?? '').toString(),
    );
  }
}

class SearchResult {
  const SearchResult({
    required this.query,
    required this.matches,
    required this.hard,
    required this.explanation,
  });

  final String query;
  final List<SearchHit> matches;
  final List<SearchHit> hard;
  final String explanation;

  factory SearchResult.fromJson(Map<String, dynamic> j) {
    List<SearchHit> read(dynamic raw) {
      if (raw is! List) return const [];
      return raw.map((e) => SearchHit.fromJson(asMap(e))).toList();
    }

    return SearchResult(
      query: (j['query'] ?? '').toString(),
      matches: read(j['matches']),
      hard: read(j['hard']),
      explanation: (j['explanation'] ?? '').toString(),
    );
  }
}
