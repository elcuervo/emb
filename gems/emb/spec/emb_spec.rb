# frozen_string_literal: true

require "spec_helper"

RSpec.describe Emb do
  before(:all) { Emb.setup(port: 16379) }
  after(:all) { Emb.reset_registry! }

  describe ".[]" do
    it "returns a proxy" do
      expect(Emb[:test]).to be_a(Emb::Proxy)
    end

    it "memoizes the proxy" do
      expect(Emb[:test]).to equal(Emb[:test])
    end
  end

  describe ".ping" do
    it "returns PONG" do
      expect(Emb.ping).to eq("PONG")
    end
  end

  describe ".models" do
    it "returns an array of model hashes" do
      models = Emb.models
      expect(models).to be_an(Array)
      expect(models.first).to have_key(:name)
      expect(models.first).to have_key(:dim)
      expect(models.first).to have_key(:status)
    end
  end

  describe ".info" do
    it "returns model info as a hash" do
      info = Emb.info(:minilm)
      expect(info).to be_a(Hash)
      expect(info[:dim]).to eq(384)
    end
  end

  describe ".help" do
    it "returns help text" do
      help = Emb.help
      expect(help).to be_a(String)
      expect(help).to include("EMB")
    end
  end

  describe "Proxy" do
    it "embeds text and returns binary data" do
      result = Emb[:minilm]["hello world"]
      expect(result).to be_a(Array)
      expect(result.size).to eq(384)
    end

    it "embeds multiple texts" do
      results = Emb[:minilm]["hello", "world"]
      expect(results).to be_an(Array)
      expect(results.length).to eq(2)
    end
  end

  describe ".multi" do
    it "sends EMB.MULTI and returns array" do
      results = Emb.multi do |m|
        m[:minilm]["hello"]
        m[:minilm]["world"]
      end
      expect(results).to be_an(Array)
      expect(results.length).to eq(2)
    end
  end
end
